---
name: shop
description: >
  Multi-store shopping CLI. Search products, check prices, compare items, read reviews, manage
  carts, and place orders via the `shop` CLI. Use when asked to buy something, find a product,
  look up prices, add to cart, checkout, order, search for products, compare products, check
  availability, look at reviews, find deals, or any shopping/e-commerce task. Currently supports
  Amazon (US/UK/DE/JP/CA/AU). Provider architecture supports adding new stores.
---

# shop

All output JSON to stdout, errors JSON to stderr. Pipe with `jq`.

**All prices are cents** (minor units). $29.99 = 2999. JPY = whole yen.

## Auth

Auth is per-store. Amazon uses a device code flow — two calls to `shop login`:

```bash
# 1. Start — returns challenge URL + code for user to enter
shop login amazon
# → .challenge.url and .challenge.code — user must visit URL and enter code

# 2. Complete — run same command again after user authorizes
shop login amazon
# → .authenticated = true
```

```bash
shop whoami -s amazon                    # check auth state
shop logout amazon                       # revoke + clear tokens
```

Auth persists in `~/.config/shop/auth/`. Most commands require auth.
On `auth_required` (exit 10) or `auth_expired` (exit 11), re-run login flow.

## Global Flags

```
-s, --store <name>   # target store (default: config or $SHOP_STORE)
--json               # force compact JSON
--pretty             # force pretty JSON
--timeout <dur>      # request timeout (default 30s)
```

Set a default store: `shop config set defaults.store amazon`

## Commands

### Search

```bash
shop search "protein powder"
shop search "usb-c cable" --sort price_low --page 2
shop search "headphones" --min-price 2000 --max-price 10000 --min-rating 4.0
```

Flags: `--sort` (relevance|price_low|price_high|rating|newest|best_seller), `--page`, `--page-size`, `--min-price`, `--max-price` (cents), `--min-rating`, `--category`, `--filter key=value` (repeatable)

Response: `.products[]` has id, title, price, rating, url, badge, availability. `.hasMore` for pagination.

### Product Details

```bash
shop product B0D1XD1ZV3
```

Product IDs are ASINs (10-char alphanumeric). Returns title, brand, price, listPrice, rating, images, specs, features, description, seller, availability, variantInfo.

### Variants

```bash
shop variants B0D1XD1ZV3
```

Returns `.dimensions[]` (Color, Size, etc.) with options, `.combinations[]` mapping dimension values → productId + price.

### Reviews

```bash
shop reviews B0D1XD1ZV3
shop reviews B0D1XD1ZV3 --sort recent --rating 5 --page 2
```

Flags: `--sort` (recent|helpful|rating), `--rating` (1-5), `--page`, `--page-size`

### Offers

```bash
shop offers B0D1XD1ZV3
shop offers B0D1XD1ZV3 --condition used_good
```

Flags: `--condition` (new|used_like_new|used_good|used_fair|refurbished), `--page`, `--page-size`

### Cart

```bash
shop cart add B0D1XD1ZV3             # add 1 unit
shop cart add B0D1XD1ZV3 --qty 3     # add 3
shop cart view                        # view cart
shop cart remove B0D1XD1ZV3           # remove item
shop cart clear                       # empty cart
```

All cart commands return full cart snapshot: `.items[]` (product + quantity) and `.subtotal`.

### Checkout (preview only)

```bash
shop checkout
shop checkout --address <id> --payment <id> --shipping <id> --coupon <code>
```

Returns checkoutId, items, subtotal, shipping, tax, discount, total, shippingAddress, paymentMethod, shippingOptions, estimatedDelivery, warnings. **Does NOT place the order.**

### Place Order

```bash
shop order place <checkout-id>
```

⚠️ **IRREVERSIBLE. Spends real money. Always confirm with the user before running.**

Fails with `cart_changed` (exit 41) if cart was modified after checkout preview.

### Account

```bash
shop addresses     # saved shipping addresses
shop payments      # saved payment methods
```

### Stores

```bash
shop stores                      # list all known stores
shop store info        # store details + capabilities
```

## Search → Buy Pipeline

```bash
# 1. Search
ASIN=$(shop search "thing" | jq -r '.products[0].id')

# 2. Inspect
shop product "$ASIN"

# 3. Cart
shop cart clear
shop cart add "$ASIN"

# 4. Preview
CID=$(shop checkout | jq -r '.checkoutId')

# 5. Place (CONFIRM WITH USER FIRST)
shop order place "$CID"
```

## jq Patterns

```bash
# top 5 results: title + price in dollars
shop search "query" | jq '.products[:5][] | {title, price_usd: (.price.amount / 100)}'

# cheapest result
shop search "query" --sort price_low | jq '.products[0] | {id, title, price: .price.amount}'

# check availability
shop product ASIN | jq '.availability.status'

# extract checkout total in dollars
shop checkout | jq '{total: (.total.amount / 100), currency: .total.currency}'

# list address IDs
shop addresses | jq '.[].id'

# is authenticated?
shop whoami | jq '.authenticated'
```

## Error Handling

Errors JSON on stderr: `{"code": "...", "message": "..."}`. Key exit codes:

- 10 `auth_required` / 11 `auth_expired` — re-login
- 30 `not_found` — bad ASIN / 31 `out_of_stock` — unavailable
- 40 `cart_empty` / 41 `cart_changed` — re-run checkout before placing
- 50 `rate_limited` — back off and retry

## Gotchas

- Prices are **always cents**. `--min-price 2000` = $20.00. Display: divide by 100.
- `order place` is **irreversible** — real charge, no undo.
- `addresses` and `payments` require items in cart to return results (Amazon-specific quirk).
- Cart state is per-store, persisted locally. `cart clear` before starting a new purchase flow.
- Amazon only returns the buy-box offer from `offers` (single seller). Other providers may differ.
- `checkout` returns a `checkoutId` tied to cart state — if anything changes, re-checkout.
