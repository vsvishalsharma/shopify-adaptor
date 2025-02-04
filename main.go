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
		Domain        string `json:"domain"`
		Action        string `json:"action"`
		Country       string `json:"country"`
		City          string `json:"city"`
		CoreVersion   string `json:"core_version"`
		BapID         string `json:"bap_id"`
		BapURI        string `json:"bap_uri"`
		TransactionID string `json:"transaction_id"`
		MessageID     string `json:"message_id"`
		Timestamp     string `json:"timestamp"`
		TTL           string `json:"ttl"`
	} `json:"context"`
	Message struct {
		Intent struct {
			Category struct {
				ID string `json:"id"`
			} `json:"category"`
			Fulfillment struct {
				Type string `json:"type"`
				City struct {
					Code string `json:"code"`
				} `json:"city"`
			} `json:"fulfillment"`
			Payment struct {
				FinderFeeType   string `json:"@ondc/org/buyer_app_finder_fee_type"`
				FinderFeeAmount string `json:"@ondc/org/buyer_app_finder_fee_amount"`
			} `json:"payment"`
			Tags []struct {
				Code string `json:"code"`
				List []struct {
					Code  string `json:"code"`
					Value string `json:"value"`
				} `json:"list"`
			} `json:"tags"`
		} `json:"intent"`
	} `json:"message"`
}

type ONDCCatalog struct {
	Fulfillments []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"bpp/fulfillments"`
	Descriptor struct {
		Name      string   `json:"name"`
		Symbol    string   `json:"symbol"`
		ShortDesc string   `json:"short_desc"`
		LongDesc  string   `json:"long_desc"`
		Images    []string `json:"images"`
		Tags      []struct {
			Code string `json:"code"`
			List []struct {
				Code  string `json:"code"`
				Value string `json:"value"`
			} `json:"list"`
		} `json:"tags"`
	} `json:"bpp/descriptor"`
	Providers []struct {
		ID    string `json:"id"`
		Items []struct {
			ID         string `json:"id"`
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

func main() {
	http.HandleFunc("/search", searchHandler)
	log.Println("Server started on :9090")
	log.Fatal(http.ListenAndServe(":9090", nil))
}

// searchHandler decodes the ONDC search request, sends an immediate ACK,
// and then asynchronously processes the search by querying Shopify.
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

// processSearch uses the search details from the ONDC request (e.g., the city code)
// to query Shopify and then transform and send the resulting catalog.
func processSearch(req ONDCSearchRequest) {
	cityCode := req.Message.Intent.Fulfillment.City.Code
	log.Printf("Processing search for city: %s", cityCode)

	// Query Shopify for products using a tag-based search.
	products := queryShopify(cityCode)

	if len(products) == 0 {
		log.Printf("No products found for search term: %s", cityCode)
	}

	// Transform the Shopify products into an ONDC catalog format.
	catalog := transformToONDCCatalog(products)
	
	if err := sendOnSearch(req.Context.BapURI, catalog, req.Context.MessageID); err != nil {
		log.Printf("Failed to send on_search for message ID %s: %v", req.Context.MessageID, err)
	}
}

// queryShopify constructs and sends a GraphQL query to Shopify to retrieve
// products that have a tag matching the search term (cityCode).
func queryShopify(cityCode string) []shopifyProduct {
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
	// Use a GraphQL query that filters products based on a tag.
	gqlQuery := fmt.Sprintf(`{
		products(first: 10, query: "tag:%s") {
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

	reqBody := struct {
		Query string `json:"query"`
	}{Query: gqlQuery}

	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Error marshaling GraphQL request body: %v", err)
		return nil
	}

	httpReq, err := http.NewRequest("POST", fullShopifyURL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating Shopify request: %v", err)
		return nil
	}
	httpReq.Header.Add("Content-Type", "application/json")
	httpReq.Header.Add("X-Shopify-Access-Token", accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Shopify request failed: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		log.Printf("Shopify API returned non-200 status: %s", resp.Status)
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

// transformToONDCCatalog converts the list of Shopify products into the ONDC catalog format.
func transformToONDCCatalog(products []shopifyProduct) ONDCCatalog {
	catalog := ONDCCatalog{
		Providers: []struct {
			ID    string `json:"id"`
			Items []struct {
				ID         string `json:"id"`
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
			ID         string `json:"id"`
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

// sendOnSearch sends the ONDC on_search response to the BAP's on_search endpoint.
// This version pretty prints the JSON payload before logging it.
func sendOnSearch(bapURI string, catalog ONDCCatalog, req ONDCSearchRequest) error {
	response := struct {
		Context struct {
			Domain        string `json:"domain"`
			Country       string `json:"country"`
			City          string `json:"city"`
			Action        string `json:"action"`
			CoreVersion   string `json:"core_version"`
			BapID         string `json:"bap_id"`
			BapURI        string `json:"bap_uri"`
			BppID         string `json:"bpp_id"`
			BppURI        string `json:"bpp_uri"`
			TransactionID string `json:"transaction_id"`
			MessageID     string `json:"message_id"`
			Timestamp     string `json:"timestamp"`
		} `json:"context"`
		Message struct {
			Catalog ONDCCatalog `json:"catalog"`
		} `json:"message"`
	}{
		Context: struct {
			Domain        string `json:"domain"`
			Country       string `json:"country"`
			City          string `json:"city"`
			Action        string `json:"action"`
			CoreVersion   string `json:"core_version"`
			BapID         string `json:"bap_id"`
			BapURI        string `json:"bap_uri"`
			BppID         string `json:"bpp_id"`
			BppURI        string `json:"bpp_uri"`
			TransactionID string `json:"transaction_id"`
			MessageID     string `json:"message_id"`
			Timestamp     string `json:"timestamp"`
		}{
			Domain:        "ONDC:RET10138",
			Country:       req.Context.Country,
			City:          req.Context.City,
			Action:        "on_search",
			CoreVersion:   "1.2.0",
			BapID:         req.Context.BapID,
			BapURI:        req.Context.BapURI,
			BppID:         os.Getenv("BPP_ID"),
			BppURI:        os.Getenv("BPP_URI"),
			TransactionID: req.Context.TransactionID,
			MessageID:     req.Context.MessageID,
			Timestamp:     time.Now().Format(time.RFC3339),
		},
		Message: struct {
			Catalog ONDCCatalog `json:"catalog"`
		}{
			Catalog: catalog,
		},
	}

	// Format the JSON payload with indentation for easier reading.
	prettyPayload, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal on_search response: %v", err)
	}

	log.Printf("Sending on_search to %s with payload: %s", bapURI+"/on_search", string(payload))

	log.Printf("Sending on_search to %s with payload:\n%s", bapURI+"/on_search", string(prettyPayload))

	resp, err := http.Post(bapURI+"/on_search", "application/json", bytes.NewBuffer(prettyPayload))
	if err != nil {
		return fmt.Errorf("failed to send on_search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("on_search failed with status: %s", resp.Status)
	}

	log.Printf("Successfully sent on_search response for message ID: %s", req.Context.MessageID)
	return nil
}
