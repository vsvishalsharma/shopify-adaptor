package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// transformToONDCCatalog converts Shopify products into the ONDC catalog format.
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
				ID:    "P1",
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

	// Create items from Shopify products.
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

	// Populate the context using the incoming request and environment variables.
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

	// Format the JSON payload with indentation.
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
