---
name: shop
description: >
  Multi-store shopping CLI. Search products, check prices, compare items, read reviews, manage
  carts, place orders, and track packages via the `shop` CLI. Use when asked to buy something, find a product,
  look up prices, add to cart, checkout, order, search for products, compare products, check
  availability, look at reviews, find deals, check a shipment, or save package tracking numbers. Currently supports
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

Auth persists in `~/.config/shop/auth/`. Store operations may require auth. Shipment tracking and its local ledger do not.
On `auth_required` (exit 10) or `auth_expired` (exit 11), re-run login flow.

## Global Flags

```
-s, --store <name>   # target store (default: config or $SHOP_STORE)
--json               # force compact JSON
--pretty             # force pretty JSON
--timeout <dur>      # request timeout (default 30s)
--config <path>      # config and local state directory
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

Use when a product has multiple options (colors, sizes, styles) and you need to show them or let the user pick one.

`shop product` tells you which variant you're currently looking at via `variantInfo.selected` (e.g. `{"Color": "Black"}`). To get the full option tree and all other ASINs, call `shop variants`.

```bash
shop variants B0D1XD1ZV3
```

Response:
- `.dimensions[]` — each axis of variation (Color, Size, etc.) with all available `.options[].value` strings and optional `.options[].imageUrl` swatches
- `.combinations[]` — every variant as `{ productId, values: {"Color": "Black"}, price, available }`

```bash
# Show all color options + their ASINs
shop variants B0C33XXS56 | jq '[.combinations[] | {asin: .productId, color: .values.Color, price: (.price.amount / 100)}]'
# → [{"asin":"B0C33XXS56","color":"Black","price":208.99}, ...]

# Get ASIN for a specific color
shop variants B0C33XXS56 | jq -r '.combinations[] | select(.values.Color == "Silver") | .productId'
```

If the user asks about a product with variants (e.g. "does it come in white?", "show me all sizes"), call `shop variants` and present the options cleanly — don't just show the ASIN they happened to land on.

### Reviews

```bash
shop reviews B0D1XD1ZV3
shop reviews B0D1XD1ZV3 --sort recent --rating 5 --page 2
```

Flags: `--sort` (recent|helpful for Amazon), `--rating` (1-5), `--page` (1-100), `--page-size` (1-100, default 10).

Review bodies use the full text supplied by Amazon. Preserve `attributes.possiblyTruncated` when present; do not call that body complete. Aggregate ratings are product-wide even when filtering reviews. Use `hasMore` to decide whether to request another page. Pagination follows a live feed and later pages take additional requests; errors are not evidence that a product has no reviews.

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
shop cart add B0D1XD1ZV3 --offer '<offer-id>' # add a specific result from `shop offers`
shop cart view                        # view cart
shop cart remove B0D1XD1ZV3           # remove item
shop cart clear                       # empty cart
```

All cart commands return full cart snapshot: `.items[]` (product + quantity) and `.subtotal`.
`--offer` accepts a provider-neutral offer ID returned by `shop offers` for the
same product. Omit it to use the provider-selected default offer.

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

### Shipment Tracking

Use for package status and scan history. No login or `--store` is needed.

```bash
shop track <tracking-number>
shop track <tracking-number> | jq '{trackingNumber, source, latest: .events[0], url}'
```

Response: `trackingNumber`, `source`, `url`, `fetchedAt`, `freshness`, and `events[]` with `date`, optional `time`, `description`, and optional `location`.

To remember what a package belongs to, save its attribution separately:

```bash
shop track add <tracking-number> --label "Desk equipment" \
  --merchant "Example Store" --order-id "ORDER001" --note "Office delivery"
shop track list
shop track refresh                    # Refresh all saved shipments
shop track refresh <tracking-number>  # Refresh one saved shipment
shop track remove <tracking-number>
```

`add` returns the saved entry with `addedAt`; all attribution flags are optional. `list` returns an array of entries without network access. `remove` returns `{"removed":true}` and is idempotent. Duplicate additions return `invalid_input` and preserve the existing entry. A lookup never saves an entry automatically; local attribution is not sent upstream.

For a named saved package, run `track list`, match its label/merchant/order ID, then look up its `trackingNumber`. Ask which package if multiple entries match; do not guess. Do not invent attribution or overwrite an existing record by removing/re-adding it unless requested.

Report the latest scan's description, date/time, location, and tracking link. Only report an ETA if the source explicitly supplies it; distinguish an estimate from a guarantee. `fetchedAt` is lookup time, not carrier freshness. `freshness: "unknown"` means freshness is unverified. Do not assume a timezone for event times or promise automatic monitoring.

#### Refresh summaries

Each shipment is stored as one JSON record in `state/shipments/<tracking-number>.json`, with attribution and a `history` field holding the last successful lookup and its scan events. `track list` reads these records offline. Older split history records are merged automatically.

`shop track refresh` looks up saved shipments and returns `{total, refreshed, failed, shipments}`. Each shipment includes its saved attribution, `latest` scan, `fetchedAt`, tracking URL, `freshness`, and `refreshed` flag. Failed entries include a structured `error` and retain the previous successful snapshot when one exists; never present those as newly refreshed.

Successful histories are saved separately from attribution in shared local state. Ordinary `shop track <tracking-number>` remains a one-off lookup. Refreshing an unsaved number returns `not_found`. An empty ledger returns an empty summary without network requests.

The timeout applies to the whole batch. Partial failures still produce the summary on stdout and return exit 51 with an error on stderr. A successful refresh means the source responded successfully, not that it contacted the carrier just now. No background polling is started.

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
- 50 `rate_limited` — back off and retry; honor `details.retryAfter` when supplied
- 51 `upstream_error` — tracking history unavailable or unexpected source response; report the failure, not “invalid tracking number”
- 2 `invalid_input` — check arguments; duplicate saved shipments leave existing metadata unchanged

## Presenting Results to Users

When showing a product, don't dump JSON. Format it for humans.

**In markdown-capable surfaces (Discord, Telegram, etc.):**

```
**[Sony WF-1000XM5](https://amazon.com/dp/B0C33XXS56)** — $248.00
⭐ 4.0 (5,813 reviews) · In Stock · Prime
https://m.media-amazon.com/images/I/example.jpg
```

- Title is a markdown link to `.url`
- Price: divide cents by 100, format as `$X.XX`
- Rating: `⭐ X.X (N reviews)`
- Image: post `.imageUrl` as a bare URL on its own line — Discord and Telegram auto-embed it
- Availability + badge on one line, only if relevant

**Search results (multiple items):** 3–5 max unless asked for more. One line each — title-link + price + rating. Only pull full product details if the user wants to dig in.

```
1. **[Sony WF-1000XM5](url)** — $248.00 · ⭐ 4.0 (5,813)
2. **[Sony WF-1000XM6](url)** — $299.00 · ⭐ 4.5 (192) 🔥 Deal
3. **[Technics EAH-AZ100](url)** — $249.00 · ⭐ 4.0 (1,221)
```

**Cart:**
```
🛒 2 items · $547.00
• [Sony WF-1000XM5](url) × 1 — $248.00
• [Apple AirPods Pro](url) × 2 — $299.00
```

**Checkout preview:**
```
📦 Order Summary
[Sony WF-1000XM5](url) × 1 — $248.00
Shipping: Free (Prime, arrives Wed Apr 2)
Tax: $22.32
**Total: $270.32**
📍 John D. · 123 Main St, New York NY 10001
💳 Visa ····4242
```
Always show this and ask for explicit confirmation before running `order place`.

**Order placed:**
```
✅ Order placed — #113-4567890-1234567
[Sony WF-1000XM5](url) × 1 — $270.32
Estimated delivery: Wed, Apr 2
```

**Reviews (when asked):**
```
⭐⭐⭐⭐⭐ **Great ANC, comfortable fit** — verified purchase
> "Best earbuds I've owned. ANC is incredible for commuting..."
— JohnD · 2 days ago · 47 helpful
```
Show 2–3 reviews max by default, top helpful first.

**In plain-text surfaces:** same patterns, skip markdown links, bare URL on its own line for images.

**Fields worth surfacing:** title, price, rating, availability, badge, url, imageUrl.
**Omit by default:** listPrice (show only if discount > 10%), specs, description, features — surface on request.

## Gotchas

- Prices are **always cents**. `--min-price 2000` = $20.00. Display: divide by 100.
- `order place` is **irreversible** — real charge, no undo.
- `addresses` and `payments` require items in cart to return results (Amazon-specific quirk).
- Cart state is per-store, persisted locally. `cart clear` before starting a new purchase flow.
- Amazon only returns the buy-box offer from `offers` (single seller). Other providers may differ.
- `checkout` returns a `checkoutId` tied to cart state — if anything changes, re-checkout.
