package checkoutmedia

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type ConvertRequest struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	Filename    string `json:"filename"`
	File        string `json:"file"`
	Format      string `json:"format"`
	Fulfillment string `json:"fulfillment"`
}

type OrderReceipt struct {
	OrderID        string `json:"order_id"`
	CustomerID     string `json:"customer_id"`
	ObjectKey      string `json:"object_key"`
	Format         string `json:"format"`
	CheckoutStatus string `json:"checkout_status"`
	Fulfillment    string `json:"fulfillment"`
	Receipt        string `json:"receipt"`
	CustomerUpdate string `json:"customer_update"`
}

type MediaStore interface {
	Upload(context.Context, string, string, string) (string, error)
	Convert(context.Context, string, string, string) (string, error)
	Put(context.Context, string, string, string, string, string) error
}

type Workflow struct {
	Store  MediaStore
	Bucket string
}

func (w Workflow) Process(ctx context.Context, in ConvertRequest) (OrderReceipt, error) {
	format := strings.ToLower(in.Format)
	if !safeID.MatchString(in.OrderID) || !safeID.MatchString(in.CustomerID) {
		return OrderReceipt{}, &ValidationError{Message: "order_id and customer_id must be path-safe"}
	}
	if in.Filename == "" || in.File == "" {
		return OrderReceipt{}, &ValidationError{Message: "filename and file are required"}
	}
	if format != "webp" && format != "avif" {
		return OrderReceipt{}, &ValidationError{Message: "format must be webp or avif"}
	}
	if in.Fulfillment != "ship" && in.Fulfillment != "pickup" {
		return OrderReceipt{}, &ValidationError{Message: "fulfillment must be ship or pickup"}
	}

	key := fmt.Sprintf("orders/%s/product.%s", in.OrderID, format)
	op := "order-image:" + in.OrderID
	uploaded, err := w.Store.Upload(ctx, in.File, filepath.Base(in.Filename), op+":upload")
	if err != nil {
		return OrderReceipt{}, err
	}
	converted, err := w.Store.Convert(ctx, uploaded, format, op+":convert")
	if err != nil {
		return OrderReceipt{}, err
	}
	if err := w.Store.Put(ctx, w.Bucket, key, converted, "image/"+format, op+":store"); err != nil {
		return OrderReceipt{}, err
	}

	return OrderReceipt{
		OrderID:        in.OrderID,
		CustomerID:     in.CustomerID,
		ObjectKey:      key,
		Format:         format,
		CheckoutStatus: "paid",
		Fulfillment:    "ready_for_" + in.Fulfillment,
		Receipt:        "receipt-" + in.OrderID,
		CustomerUpdate: "product image ready",
	}, nil
}
