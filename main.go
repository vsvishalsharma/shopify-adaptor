package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// ONDC Structures
type ONDCSearchRequest struct {
	Context struct {
		Domain    string `json:"domain"`
		BapURI    string `json:"bap_uri"`
		MessageID string `json:"message_id"`
	} `json:"context"`
	Message struct {
		Intent struct {
			Item struct {
				Descriptor struct {
					Name string `json:"name"`
				} `json:"descriptor"`
			} `json:"item"`
		} `json:"intent"`
	} `json:"message"`
}

type ONDCCatalog struct {
	Providers []struct {
		ID    string `json:"id"`
		Items []struct {
			ID        string `json:"id"`
			Descriptor struct {
				Name string `json:"name"`
			} `json:"descriptor"`
			Price struct {
				Currency string `json:"currency"`
				Value    string `json:"value"`
			} `json:"price"`
		} `json:"items"`
	} `json:"bpp/providers"`
}

// Shopify Structures
type shopifyProduct struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Price string `json:"price"`
}

func init() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Set default environment variables if not already set
	if os.Getenv("SHOPIFY_URL") == "" {
		os.Setenv("SHOPIFY_URL", "https://testgamaa.myshopify.com")
	}

	// Log configuration on startup
	log.Printf("Shopify URL: %s", os.Getenv("SHOPIFY_URL"))
	log.Printf("Access Token configured: %v", os.Getenv("SHOPIFY_ACCESS_TOKEN") != "")
}

func main() {
	http.HandleFunc("/search", searchHandler)
	log.Println("Server started on :9090")
	log.Fatal(http.ListenAndServe(":9090", nil))
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	var req ONDCSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding search request: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Send immediate ACK
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"message": map[string]interface{}{"ack": map[string]string{"status": "ACK"}},
	}); err != nil {
		log.Printf("Error sending ACK: %v", err)
	}

	// Process async
	go processSearch(req)
}

func processSearch(req ONDCSearchRequest) {
	searchTerm := req.Message.Intent.Item.Descriptor.Name
	log.Printf("Processing search for term: %s", searchTerm)

	products := queryShopify(searchTerm)
	if len(products) == 0 {
		log.Printf("No products found for search term: %s", searchTerm)
	}

	catalog := transformToONDCCatalog(products)

	if err := sendOnSearch(req.Context.BapURI, catalog, req.Context.MessageID); err != nil {
		log.Printf("Failed to send on_search for message ID %s: %v", req.Context.MessageID, err)
	}
}

func queryShopify(query string) []shopifyProduct {
	shopifyURL := os.Getenv("SHOPIFY_URL")
	accessToken := os.Getenv("SHOPIFY_ACCESS_TOKEN")

	log.Printf("Starting Shopify query with URL: %s", shopifyURL)

	if shopifyURL == "" || accessToken == "" {
		log.Printf("Error: Missing Shopify configuration - URL: %s, Access Token: %s",
			shopifyURL,
			func() string {
				if accessToken != "" {
					return "**REDACTED**"
				}
				return "EMPTY"
			}())
		return nil
	}

	fullShopifyURL := shopifyURL + "/admin/api/2025-01/graphql.json"
	gqlQuery := fmt.Sprintf(`{
		products(first: 10, query: "title:*%s*") {
			edges {
				node {
					id
					title
					variants(first: 1) {
						edges {
							node {
								price
							}
						}
					}
				}
			}
		}
	}`, query)

	log.Printf("Executing GraphQL query: %s", gqlQuery)

	reqBody := struct {
		Query string `json:"query"`
	}{Query: gqlQuery}

	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Error marshaling GraphQL request body: %v", err)
		return nil
	}

	req, err := http.NewRequest("POST", fullShopifyURL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating Shopify request: %v", err)
		return nil
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Shopify-Access-Token", accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Shopify request failed: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Read and log the raw response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil
	}
	log.Printf("Raw Shopify response: %s", string(respBody))

	var result struct {
		Data struct {
			Products struct {
				Edges []struct {
					Node struct {
						ID       string `json:"id"`
						Title    string `json:"title"`
						Variants struct {
							Edges []struct {
								Node struct {
									Price string `json:"price"`
								} `json:"node"`
							} `json:"edges"`
						} `json:"variants"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"products"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("Error decoding Shopify GraphQL response: %v", err)
		return nil
	}

	if len(result.Errors) > 0 {
		for _, gqlErr := range result.Errors {
			log.Printf("GraphQL error: %s", gqlErr.Message)
		}
		return nil
	}

	var products []shopifyProduct
	for _, edge := range result.Data.Products.Edges {
		node := edge.Node
		price := ""
		if len(node.Variants.Edges) > 0 {
			price = node.Variants.Edges[0].Node.Price
		}
		products = append(products, shopifyProduct{
			ID:    node.ID,
			Title: node.Title,
			Price: price,
		})
	}

	log.Printf("Successfully found %d products", len(products))
	return products
}

func transformToONDCCatalog(products []shopifyProduct) ONDCCatalog {
	catalog := ONDCCatalog{
		Providers: []struct {
			ID    string `json:"id"`
			Items []struct {
				ID        string `json:"id"`
				Descriptor struct {
					Name string `json:"name"`
				} `json:"descriptor"`
				Price struct {
					Currency string `json:"currency"`
					Value    string `json:"value"`
				} `json:"price"`
			} `json:"items"`
		}{
			{
				ID: "shopify-store",
			},
		},
	}

	for _, p := range products {
		item := struct {
			ID        string `json:"id"`
			Descriptor struct {
				Name string `json:"name"`
			} `json:"descriptor"`
			Price struct {
				Currency string `json:"currency"`
				Value    string `json:"value"`
			} `json:"price"`
		}{
			ID: p.ID,
			Descriptor: struct {
				Name string `json:"name"`
			}{Name: p.Title},
			Price: struct {
				Currency string `json:"currency"`
				Value    string `json:"value"`
			}{
				Currency: "INR",
				Value:    p.Price,
			},
		}
		catalog.Providers[0].Items = append(catalog.Providers[0].Items, item)
	}

	return catalog
}

func sendOnSearch(bapURI string, catalog ONDCCatalog, messageID string) error {
	response := struct {
		Context struct {
			Domain    string `json:"domain"`
			MessageID string `json:"message_id"`
		} `json:"context"`
		Message struct {
			Catalog ONDCCatalog `json:"catalog"`
		} `json:"message"`
	}{
		Context: struct {
			Domain    string `json:"domain"`
			MessageID string `json:"message_id"`
		}{
			Domain:    "nic2004:52110",
			MessageID: messageID,
		},
		Message: struct {
			Catalog ONDCCatalog `json:"catalog"`
		}{
			Catalog: catalog,
		},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal on_search response: %v", err)
	}

	log.Printf("Sending on_search to %s with payload: %s", bapURI+"/on_search", string(payload))

	resp, err := http.Post(bapURI+"/on_search", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send on_search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("on_search failed with status: %s", resp.Status)
	}

	log.Printf("Successfully sent on_search response for message ID: %s", messageID)
	return nil
}
