// Package purchases sells what an app sells in itself: StoreKit 2 on iOS, and
// Google Play Billing on Android, which their stores require for digital goods.
// Products are set up in App Store Connect and the Play Console.
//
//	products, err := purchases.Products(ctx, "pro.monthly", "coins.100")
//	p, err := purchases.Buy(ctx, products[0].ID)
//	if err == nil && p != nil {
//		// Give what was bought, then:
//		purchases.Finish(ctx, *p)
//	}
//
// Elsewhere its functions report errors.ErrUnsupported.
package purchases

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/egoist/mygo"
)

const pluginID = "purchases"

// Product is something for sale, as the store describes it to this user.
type Product struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Price is the price as the store shows it, in the user's currency.
	Price string `json:"price"`
	// PriceMicros is the price in millionths of Currency's unit.
	PriceMicros int64  `json:"priceMicros"`
	Currency    string `json:"currency"`
	// Subscription says it renews.
	Subscription bool `json:"subscription"`
}

// Purchase is what the user bought.
type Purchase struct {
	ProductID string `json:"productId"`
	// Token is what the app's server verifies the purchase with: the
	// transaction's signed JWS on iOS, the purchase token on Android.
	Token string `json:"token"`
	// Pending says the store waits for something first, as a parent's approval
	// or a payment in cash: give nothing yet.
	Pending bool `json:"pending"`
	// Finished says the app already finished it.
	Finished bool `json:"finished"`
	// Consumable, set by the app before Finish, lets the user buy it again.
	Consumable bool `json:"consumable"`
}

// ErrCanceled is what Buy fails with as the user goes back.
var ErrCanceled = errors.New("purchases: canceled")

// Products asks the store for what is for sale, by the products' IDs.
func Products(ctx context.Context, ids ...string) ([]Product, error) {
	var r struct {
		Products []Product `json:"products"`
	}
	err := mygo.Native(pluginID).Call(ctx, "products", map[string]any{"ids": ids}, &r)
	return r.Products, err
}

// Buy shows the store's sheet of a product, and returns the purchase, which the
// app finishes once it gave what was bought (Finish). Call it off the main
// goroutine, as it waits for the user.
func Buy(ctx context.Context, id string) (*Purchase, error) {
	var p Purchase
	err := mygo.Native(pluginID).Call(ctx, "buy", map[string]any{"id": id}, &p)
	var n *mygo.NativeError
	if errors.As(err, &n) && n.Code == "Canceled" {
		return nil, ErrCanceled
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Finish tells the store the app gave what was bought: without it, the store
// gives the purchase again, and Google Play refunds it in three days.
func Finish(ctx context.Context, p Purchase) error {
	return mygo.Native(pluginID).Call(ctx, "finish", p, nil)
}

// Restore lists what the user bought and still has, as on a new device.
func Restore(ctx context.Context) ([]Purchase, error) {
	var r struct {
		Purchases []Purchase `json:"purchases"`
	}
	err := mygo.Native(pluginID).Call(ctx, "restore", nil, &r)
	return r.Purchases, err
}

// OnUpdate calls fn on the main thread as a purchase comes outside Buy, as a
// subscription's renewal or one a parent approved. Finish it as well.
func OnUpdate(fn func(Purchase)) (off func()) {
	return mygo.Native(pluginID).On("purchase", func(data []byte) {
		var p Purchase
		if json.Unmarshal(data, &p) == nil {
			fn(p)
		}
	})
}
