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
				// fulfillment section is no longer used for city code as per instructions
				Type string `json:"type"`
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

func init() {
	// Load .env file if available
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

	// Process search asynchronously
	go processSearch(req)
}

// processSearch now picks the city from context.city and queries Shopify accordingly.
func processSearch(req ONDCSearchRequest) {
	city := req.Context.City
	log.Printf("Processing search for city: %s", city)

	// Query Shopify for products using a tag-based search.
	products := queryShopify(city)

	if len(products) == 0 {
		log.Printf("No products found for search term: %s", city)
	}

	// Transform the Shopify products into an ONDC catalog format with all mandatory attributes.
	catalog := transformToONDCCatalog(products)

	// Pass the entire request structure for context
	if err := sendOnSearch(req.Context.BapURI, catalog, req); err != nil {
		log.Printf("Failed to send on_search for message ID %s: %v", req.Context.MessageID, err)
	}
}

// queryShopify constructs and sends a GraphQL query to Shopify to retrieve
// products that have tags matching the search criteria.
// If the context city equals "std:080", it searches for products tagged with "Delhi" OR "080".
func queryShopify(city string) []shopifyProduct {
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

	// Set the query tag. If city is "std:080", use both tags "Delhi" OR "080"
	queryTag := city
	if city == "std:080" {
		queryTag = "Delhi OR 080"
	}

	// Use a GraphQL query that filters products based on the tag(s)
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
	}`, queryTag)

	log.Printf("Executing GraphQL query: %s", gqlQuery)

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

	// Read and log the raw response from Shopify.
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

// transformToONDCCatalog converts the list of Shopify products into the ONDC catalog format
// and includes all mandatory attributes.
func transformToONDCCatalog(products []shopifyProduct) ONDCCatalog {
    catalog := ONDCCatalog{
        Fulfillments: []struct {
            ID   string `json:"id"`
            Type string `json:"type"`
        }{
            {ID: "1", Type: "Delivery"},
            {ID: "2", Type: "Self-Pickup"},
            {ID: "3", Type: "Delivery and Self-Pickup"},
        },
        Descriptor: struct {
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
        }{
            Name:      "Seller NP",
            Symbol:    "https://sellerNP.com/images/np.png",
            ShortDesc: "Seller Marketplace",
            LongDesc:  "Seller Marketplace",
            Images:    []string{"https://sellerNP.com/images/np.png"},
            Tags: []struct {
                Code string `json:"code"`
                List []struct {
                    Code  string `json:"code"`
                    Value string `json:"value"`
                } `json:"list"`
            }{
                {
                    Code: "bpp_terms",
                    List: []struct {
                        Code  string `json:"code"`
                        Value string `json:"value"`
                    }{
                        {Code: "np_type", Value: "MSN"},
                        {Code: "accept_bap_terms", Value: "Y"},
                        {Code: "collect_payment", Value: "Y"},
                    },
                },
            },
        },
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
                ID: "P1",
                Items: []struct {
                    ID         string `json:"id"`
                    Descriptor struct {
                        Name string `json:"name"`
                    } `json:"descriptor"`
                    Price struct {
                        Currency string `json:"currency"`
                        Value    string `json:"value"`
                    } `json:"price"`
                }{},
            },
        },
    }

    // Build more comprehensive items based on products
    for i, p := range products {
        itemID := fmt.Sprintf("I%d", i+1)
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
            ID: itemID,
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
// It formats the JSON payload with indentation before sending.
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
	}{}
	// Populate context from the request and environment values.
	response.Context.Domain = req.Context.Domain
	response.Context.Country = req.Context.Country
	response.Context.City = req.Context.City
	response.Context.Action = "on_search"
	response.Context.CoreVersion = "1.2.0"
	response.Context.BapID = req.Context.BapID
	response.Context.BapURI = req.Context.BapURI
	response.Context.BppID = os.Getenv("BPP_ID")
	response.Context.BppURI = os.Getenv("BPP_URI")
	response.Context.TransactionID = req.Context.TransactionID
	response.Context.MessageID = req.Context.MessageID
	response.Context.Timestamp = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    

	response.Message.Catalog = catalog

	// Format the JSON payload with indentation for easier reading.
	prettyPayload, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal on_search response: %v", err)
	}

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
