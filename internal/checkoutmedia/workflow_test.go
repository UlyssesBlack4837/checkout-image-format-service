package checkoutmedia

import (
	"context"
	"testing"
)

type recordingStore struct {
	uploadedKey string
	format      string
	bucket      string
	storedKey   string
}

func (s *recordingStore) Upload(_ context.Context, _, _ string, key string) (string, error) {
	s.uploadedKey = key
	return "uploaded-image", nil
}

func (s *recordingStore) Convert(_ context.Context, _ string, format, _ string) (string, error) {
	s.format = format
	return "Y29udmVydGVk", nil
}

func (s *recordingStore) Put(_ context.Context, bucket, key, _, _, _ string) error {
	s.bucket, s.storedKey = bucket, key
	return nil
}

func TestProcessOrderImage(t *testing.T) {
	tests := []struct {
		name              string
		format            string
		fulfillment       string
		wantKey           string
		wantFulfillment   string
		wantCustomerState string
	}{
		{"webp shipment", "webp", "ship", "orders/order-42/product.webp", "ready_for_ship", "product image ready"},
		{"avif pickup", "avif", "pickup", "orders/order-42/product.avif", "ready_for_pickup", "product image ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStore{}
			workflow := Workflow{Store: store, Bucket: "checkout-media"}
			got, err := workflow.Process(context.Background(), ConvertRequest{
				OrderID: "order-42", CustomerID: "customer-9", Filename: "shoe.png",
				File: "cG5n", Format: tt.format, Fulfillment: tt.fulfillment,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.ObjectKey != tt.wantKey || store.storedKey != tt.wantKey {
				t.Fatalf("receipt key %q and stored key %q; want same key %q", got.ObjectKey, store.storedKey, tt.wantKey)
			}
			if store.format != tt.format || got.Fulfillment != tt.wantFulfillment || got.CustomerUpdate != tt.wantCustomerState {
				t.Fatalf("unexpected decision: %#v, converted format %q", got, store.format)
			}
			if got.CheckoutStatus != "paid" || got.Receipt != "receipt-order-42" {
				t.Fatalf("checkout receipt not completed: %#v", got)
			}
		})
	}
}
