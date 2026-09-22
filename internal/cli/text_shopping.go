package cli

import (
	"fmt"
	"strings"

	"github.com/saucesteals/shop"
)

func (o *textOutput) availability(value shop.Availability) {
	o.field("Availability", nonempty(strings.ReplaceAll(string(value.Status), "_", " "), "unknown"))
	o.field("Details", value.Message)
}

func (o *textOutput) rating(value *shop.Rating) {
	if value != nil {
		o.field("Rating", fmt.Sprintf("%.1f/5 (%d ratings)", value.Average, value.Count))
	}
}

func (o *textOutput) page(page int, more bool) {
	o.section()
	o.field("Page", fmt.Sprint(page))
	if more {
		o.field("More results", fmt.Sprintf("use --page %d", page+1))
	}
}

func (o *textOutput) search(result *shop.SearchResult) {
	o.line(fmt.Sprintf("Products (%d)", len(result.Products)))
	for i, product := range result.Products {
		o.section()
		o.line(fmt.Sprintf("%d. %s", i+1, product.Title))
		o.field("ID", product.ID)
		o.field("Price", money(product.Price))
		o.availability(product.Availability)
		o.rating(product.Rating)
		o.field("Badge", product.Badge)
		if product.Sponsored {
			o.field("Sponsored", "yes")
		}
		o.field("Link", product.URL)
	}
	for _, warning := range result.Warnings {
		o.field("Warning", warning)
	}
	o.page(result.Page, result.HasMore)
}

func (o *textOutput) product(product *shop.Product) {
	o.line(product.Title)
	o.field("ID", product.ID)
	o.field("Brand", product.Brand)
	o.field("Price", money(product.Price))
	if product.ListPrice != nil {
		o.field("List price", money(product.ListPrice))
	}
	o.availability(product.Availability)
	o.rating(product.Rating)
	if product.Seller != nil {
		o.field("Seller", product.Seller.Name)
	}
	if product.VariantInfo != nil {
		o.fields(product.VariantInfo.Selected)
	}
	o.field("Link", product.URL)
	if product.Description != "" {
		o.section()
		o.line(product.Description)
	}
	for _, feature := range product.Features {
		o.line("  - " + feature)
	}
	for _, spec := range product.Specs {
		o.field(spec.Name, spec.Value)
	}
}

func (o *textOutput) reviews(result *shop.ReviewsResult) {
	o.line(fmt.Sprintf("Reviews (%d)", len(result.Reviews)))
	o.rating(&result.Rating)
	for _, review := range result.Reviews {
		o.section()
		o.line(fmt.Sprintf("%d/5  %s", review.Rating, nonempty(review.Title, "Review")))
		o.field("Author", review.Author)
		o.field("Date", review.Date)
		o.field("Verified purchase", yesNo(review.Verified))
		o.line(review.Body)
		if truncated, _ := review.Attributes["possiblyTruncated"].(bool); truncated {
			o.field("Warning", "Review may be truncated by the source")
		}
		if review.Helpful > 0 {
			o.field("Helpful votes", fmt.Sprint(review.Helpful))
		}
	}
	o.page(result.Page, result.HasMore)
}

func (o *textOutput) offers(result *shop.OffersResult) {
	o.line(fmt.Sprintf("Offers (%d)", len(result.Offers)))
	for _, offer := range result.Offers {
		o.section()
		o.line(offer.Seller.Name)
		o.field("Offer ID", offer.ID)
		o.field("Price", money(&offer.Price))
		o.field("Condition", strings.ReplaceAll(string(offer.Condition), "_", " "))
		o.availability(offer.Availability)
		if offer.Shipping != nil {
			o.field("Shipping", money(offer.Shipping.Price))
			o.field("Shipping details", offer.Shipping.Description)
		}
		o.field("Expected delivery", offer.DeliveryDate)
		if offer.IsBuyBox {
			o.field("Featured offer", "yes")
		}
		if offer.IsPrime {
			o.field("Prime", "yes")
		}
	}
	o.page(result.Page, result.HasMore)
}

func (o *textOutput) variants(result *shop.VariantsResult) {
	o.line("Variants for " + result.ParentID)
	for _, dimension := range result.Dimensions {
		options := make([]string, 0, len(dimension.Options))
		for _, option := range dimension.Options {
			options = append(options, option.Value)
		}
		o.field(dimension.Name, strings.Join(options, ", "))
	}
	if len(result.Combinations) == 0 {
		o.line("No variant combinations returned")
	}
	for _, variant := range result.Combinations {
		o.section()
		o.line(variant.ProductID)
		o.fields(variant.Values)
		o.field("Price", money(variant.Price))
		o.field("Available", yesNo(variant.Available))
	}
	if result.Truncated {
		o.field("Warning", "Variant list is incomplete")
	}
}

func (o *textOutput) cartItems(items []shop.CartEntry) {
	if len(items) == 0 {
		o.line("No items")
	}
	for _, item := range items {
		o.section()
		o.line(fmt.Sprintf("%d x %s", item.Quantity, item.Product.Title))
		o.field("ID", item.Product.ID)
		o.field("Unit price", money(item.Product.Price))
		o.availability(item.Product.Availability)
	}
	o.section()
}

func (o *textOutput) checkout(result *shop.CheckoutResult) {
	o.line("Checkout preview - no order placed")
	o.field("Checkout ID", result.CheckoutID)
	o.cartItems(result.Items)
	o.field("Subtotal", money(&result.Subtotal))
	o.field("Shipping", money(&result.Shipping))
	o.field("Tax", money(&result.Tax))
	o.field("Discount", money(&result.Discount))
	o.field("Total", money(&result.Total))
	o.field("Expected delivery", result.EstimatedDelivery)
	o.section()
	o.address(result.ShippingAddress)
	o.payment(result.PaymentMethod)
	if result.SelectedShipping != nil {
		o.field("Selected shipping", result.SelectedShipping.Label)
	}
	for _, option := range result.ShippingOptions {
		o.field("Shipping option", fmt.Sprintf("%s / %s / %s", option.ID, option.Label, money(&option.Price)))
		o.field("Estimated date", option.EstimatedDate)
		o.field("Estimated days", option.EstimatedDays)
	}
	for _, warning := range result.Warnings {
		o.field("Warning", warning)
	}
}

func (o *textOutput) order(order *shop.Order) {
	o.line("Order " + order.OrderID)
	o.field("Status", order.Status)
	o.field("Placed", order.PlacedAt)
	o.cartItems(order.Items)
	o.field("Total", money(&order.Total))
	o.field("Expected delivery", order.EstimatedDelivery)
	o.address(order.ShippingAddress)
	o.payment(order.PaymentMethod)
}
