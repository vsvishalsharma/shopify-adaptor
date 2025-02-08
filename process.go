package main

import (
	"log"
)

// processSearch handles the search request, querying Shopify and sending on_search response
func processSearch(req ONDCSearchRequest) {
	city := req.Context.City
	log.Printf("Processing search for city: %s", city)

	// Query Shopify for products using a tag-based search
	products := queryShopify(city)

	if len(products) == 0 {
		log.Printf("No products found for search term: %s", city)
	}

	// Transform the Shopify products into an ONDC catalog format
	catalog := transformToONDCCatalog(products)

	// Pass the entire request structure for context
	if err := sendOnSearch(req.Context.BapURI, catalog, req); err != nil {
		log.Printf("Failed to send on_search for message ID %s: %v", req.Context.MessageID, err)
	}
}