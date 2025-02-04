# Shopify ONDC Adaptor

A Go-based adaptor that connects Shopify stores to the ONDC (Open Network for Digital Commerce) network. This adaptor allows Shopify merchants to list their products on ONDC by translating between Shopify's GraphQL API and ONDC's protocol.

## Prerequisites

- Go 1.19 or higher
- Shopify store with API access
- Shopify Admin API access token
- `netcat` (nc) for testing callbacks

## Configuration

### Environment Variables

The adaptor requires the following environment variables:

```bash
SHOPIFY_URL=https://your-store.myshopify.com
SHOPIFY_ACCESS_TOKEN=your_access_token
```

These can be set in your environment or will use default test values if not set.

## Installation

1. Clone the repository:
```bash
git clone [repository-url]
cd shopify-ondc-adaptor
```

2. Install dependencies:
```bash
go mod download
```

## Running the Server

Start the server:
```bash
go run main.go
```

The server will start on port 9090.

## Testing

### 1. Start the Callback Listener

Open a terminal and start a netcat listener to simulate the ONDC callback endpoint:
```bash
nc -l -p 8082
```

### 2. Send a Test Request

In another terminal, send a test search request:
```bash
curl -X POST http://localhost:9090/search \
-H "Content-Type: application/json" \
-d '{
  "context": {
    "domain": "nic2004:52110",
    "bap_uri": "http://localhost:8082",
    "message_id": "123456"
  },
  "message": {
    "intent": {
      "item": {
        "descriptor": {
          "name": "Example T-Shirt"
        }
      }
    }
  }
}'
```

### 3. Verify the Responses

You should see:
1. An immediate ACK response from the /search endpoint
2. Server logs showing the Shopify GraphQL query
3. The callback response in your netcat terminal

### Test with Different Products

Test searching for different products by modifying the `name` field in the request:
```bash
curl -X POST http://localhost:9090/search \
-H "Content-Type: application/json" \
-d '{
  "context": {
    "domain": "nic2004:52110",
    "bap_uri": "http://localhost:8082",
    "message_id": "123456"
  },
  "message": {
    "intent": {
      "item": {
        "descriptor": {
          "name": "YOUR_PRODUCT_NAME"
        }
      }
    }
  }
}'
```

### Testing the Shopify GraphQL API Directly

You can test the Shopify GraphQL API directly using curl:
```bash
curl -X POST "https://your-store.myshopify.com/admin/api/2025-01/graphql.json" \
  -H "Content-Type: application/json" \
  -H "X-Shopify-Access-Token: your_access_token" \
  -d '{
    "query": "{ products(first: 10, query: \"title:*Example T-Shirt*\") { edges { node { id title variants(first: 1) { edges { node { price } } } } } } }"
  }'
```

## Troubleshooting

### Common Issues

1. **Empty Product List**
   - Verify your Shopify store has products
   - Check the search term matches product titles
   - Verify the GraphQL query response in the logs

2. **Connection Refused on Callback**
   - Ensure netcat is running on port 8082
   - Check the bap_uri in your request matches the netcat port

3. **Authentication Errors**
   - Verify your SHOPIFY_ACCESS_TOKEN is correct
   - Check the token has the necessary permissions

### Debugging

The server provides detailed logs for debugging:
- Incoming search requests
- GraphQL queries sent to Shopify
- Raw responses from Shopify
- Callback attempts and responses

Monitor these logs in the terminal where you run `go run main.go`.

## API Reference

### Search Endpoint

`POST /search`

Request body:
```json
{
  "context": {
    "domain": "nic2004:52110",
    "bap_uri": "string",
    "message_id": "string"
  },
  "message": {
    "intent": {
      "item": {
        "descriptor": {
          "name": "string"
        }
      }
    }
  }
}
```

Response:
```json
{
  "message": {
    "ack": {
      "status": "ACK"
    }
  }
}
```

## Contributing

Please submit issues and pull requests for any improvements you'd like to make.

## License

[Your License Here]
