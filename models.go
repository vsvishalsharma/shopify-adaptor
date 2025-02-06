package main

// ONDC Structures

// ONDCSearchRequest represents the search request payload from ONDC.
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

// ONDCCatalog represents the catalog structure to be sent in on_search.
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

// shopifyProduct represents a product returned from Shopify.
type shopifyProduct struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Price string `json:"price"`
}
