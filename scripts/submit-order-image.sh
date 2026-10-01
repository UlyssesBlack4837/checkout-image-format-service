#!/bin/sh
set -eu

curl --fail-with-body http://localhost:8080/orders/image \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"order-42","customer_id":"customer-9","filename":"shoe.png","file":"cG5n","format":"avif","fulfillment":"pickup"}'
