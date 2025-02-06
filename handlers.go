package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
)

// searchHandler decodes the ONDC search request, sends an immediate ACK,
// and then processes the search asynchronously.
func searchHandler(w http.ResponseWriter, r *http.Request) {
    var req ONDCSearchRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("Error decoding search request: %v", err)
        http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
        return
    }

    // Send immediate ACK.
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(map[string]interface{}{
        "message": map[string]interface{}{"ack": map[string]string{"status": "ACK"}},
    }); err != nil {
        log.Printf("Error sending ACK: %v", err)
    }

    // Process search asynchronously.
    go processSearch(req)
}
