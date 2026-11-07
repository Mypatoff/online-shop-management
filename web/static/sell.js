// Sell page: search an active product, pick a quantity, and sell it.

var allProducts = [];
var selected = null;

var searchInput = document.getElementById("sell-search");
var resultsList = document.getElementById("sell-results");
var selectedPanel = document.getElementById("sell-selected");
var selectedNameEl = document.getElementById("selected-name");
var selectedPriceEl = document.getElementById("selected-price");
var qtyInput = document.getElementById("qty-input");
var cupsInput = document.getElementById("cups-input");
var cupsPriceEl = document.getElementById("cups-price");
var cupPrice = 0; // what one cup (stakan) costs, set with the "Set price" button
var MAX_CUPS = 1000;
var totalEl = document.getElementById("sell-total");
var sellBtn = document.getElementById("sell-btn");
var sellErrorEl = document.getElementById("sell-error");

window.configReady.then(function () {
	loadProducts();
	loadRecentSales();
	loadCupPrice();
});

searchInput.addEventListener("input", function () {
	renderResults(searchInput.value.trim());
});

document.getElementById("qty-dec").onclick = function () {
	setQuantity(currentQuantity() - 1);
};
document.getElementById("qty-inc").onclick = function () {
	setQuantity(currentQuantity() + 1);
};
qtyInput.addEventListener("input", function () {
	setQuantity(currentQuantity());
});

document.getElementById("cups-dec").onclick = function () {
	setCups(currentCups() - 1);
};
document.getElementById("cups-inc").onclick = function () {
	setCups(currentCups() + 1);
};
cupsInput.addEventListener("input", function () {
	setCups(currentCups());
});
document.getElementById("cup-price-btn").onclick = openCupPriceModal;

sellBtn.onclick = function () {
	if (!selected) return;
	var quantity = currentQuantity();
	var cups = currentCups();
	var productID = selected.id;
	sellErrorEl.textContent = "";
	sellBtn.disabled = true;

	apiFetch("/api/sales", {
		method: "POST",
		body: JSON.stringify({ product_id: selected.id, quantity: quantity, cups: cups }),
	})
		.then(function (res) {
			showToast(
				cups > 0
					? t("sell.sold_cups", { qty: quantity, name: selected.name, cups: cups, left: res.stock })
					: t("sell.sold", { qty: quantity, name: selected.name, left: res.stock })
			);
			setCups(0); // cups belong to one sale; the next starts at none
			updateProductStock(selected.id, res.stock);
			selectProduct(selected);
			loadRecentSales();
		})
		.catch(function (err) {
			sellErrorEl.textContent = err.message;
			// The sale may have failed because stock changed underneath
			// us (e.g. sold out on another tab): reload products and
			// re-select so the stock number and quantity stepper are
			// trustworthy again, not just whatever they showed before.
			return loadProducts().then(function () {
				var fresh = allProducts.find(function (item) {
					return item.id === productID;
				});
				if (fresh) selectProduct(fresh);
			});
		})
		.finally(function () {
			// Driven by actual stock, not unconditionally re-enabled: a
			// sold-out product must stay disabled even after this sale
			// (or reload) finishes.
			sellBtn.disabled = !selected || selected.stock <= 0;
		});
};

function loadProducts() {
	return apiFetch("/api/products?archived=0&sort=popular").then(function (products) {
		allProducts = products;
		renderResults(searchInput.value.trim());
	});
}

function renderResults(query) {
	var needle = query.toLowerCase();
	var matches = allProducts.filter(function (p) {
		return needle === "" || p.name.toLowerCase().indexOf(needle) !== -1 || (p.sku || "").toLowerCase().indexOf(needle) !== -1;
	});

	resultsList.textContent = "";
	matches.forEach(function (p) {
		var li = document.createElement("li");
		var btn = document.createElement("button");
		btn.type = "button";
		btn.className = "sell-result-btn";
		btn.disabled = p.stock <= 0;
		btn.textContent = p.name + (p.sku ? " (" + p.sku + ")" : "") + " — " + formatMoney(p.price) + " — " + t("sell.in_stock", { n: p.stock });
		btn.onclick = function () {
			selectProduct(p);
		};
		li.appendChild(btn);
		resultsList.appendChild(li);
	});
}

function selectProduct(p) {
	// Cups belong to the sale being built: picking a different product starts over.
	if (!selected || selected.id !== p.id) cupsInput.value = 0;
	// Use the freshest copy in allProducts (stock may have just changed).
	selected = allProducts.find(function (item) {
		return item.id === p.id;
	}) || p;

	selectedPanel.classList.remove("hidden");
	selectedNameEl.textContent = selected.name;
	selectedPriceEl.textContent = t("sell.each", { v: formatMoney(selected.price) });
	sellErrorEl.textContent = "";

	var clamped = Math.min(Math.max(currentQuantity() || 1, 1), Math.max(selected.stock, 0));
	qtyInput.value = selected.stock > 0 ? Math.max(clamped, 1) : 0;
	qtyInput.max = selected.stock;
	sellBtn.disabled = selected.stock <= 0;
	updateTotal();
}

function currentQuantity() {
	var n = parseInt(qtyInput.value, 10);
	return isNaN(n) ? 1 : n;
}

function setQuantity(n) {
	if (!selected) return;
	var max = Math.max(selected.stock, 1);
	n = Math.min(Math.max(n, 1), max);
	qtyInput.value = n;
	updateTotal();
}

function currentCups() {
	var n = parseInt(cupsInput.value, 10);
	if (isNaN(n) || n < 0) return 0;
	return Math.min(n, MAX_CUPS);
}

function setCups(n) {
	n = Math.min(Math.max(n, 0), MAX_CUPS);
	cupsInput.value = n;
	updateTotal();
}

// The total is what the customer pays: the product(s) plus any cups.
function updateTotal() {
	if (!selected) {
		totalEl.textContent = "—";
		return;
	}
	totalEl.textContent = formatMoney(selected.price * currentQuantity() + cupPrice * currentCups());
}

function loadCupPrice() {
	return apiFetch("/api/cup-price")
		.then(function (res) {
			cupPrice = res.price;
			showCupPrice();
			updateTotal();
		})
		.catch(function () {});
}

function showCupPrice() {
	cupsPriceEl.textContent = cupPrice > 0 ? t("sell.each", { v: formatMoney(cupPrice) }) : t("sell.cup_free");
}

// priceForInput shows a stored price in an input without the currency label.
function priceForInput(amount) {
	var cfg = window.appConfig || { currency: "" };
	var text = formatMoney(amount);
	return cfg.currency ? text.slice(0, text.length - cfg.currency.length - 1) : text;
}

function openCupPriceModal() {
	openModal({
		title: t("sell.cup_price_title"),
		submitLabel: t("common.save"),
		fields: [{ name: "price", label: t("sell.cup_price_label"), value: priceForInput(cupPrice), autofocus: true }],
		onSubmit: function (values) {
			var price;
			try {
				price = parseMoney(values.price);
			} catch (err) {
				throw fieldError("price", err.message);
			}
			if (price < 0 || price > 100000000000) throw fieldError("price", t("products.err_price"));
			return apiFetch("/api/cup-price", { method: "PUT", body: JSON.stringify({ price: price }) }).then(function (res) {
				cupPrice = res.price;
				showCupPrice();
				updateTotal();
			});
		},
	});
}

function updateProductStock(id, newStock) {
	var p = allProducts.find(function (item) {
		return item.id === id;
	});
	if (p) p.stock = newStock;
}

function loadRecentSales() {
	apiFetch("/api/sales?limit=10").then(renderRecentSales);
}

function renderRecentSales(sales) {
	var list = document.getElementById("recent-sales-list");
	var empty = document.getElementById("recent-sales-empty");
	list.textContent = "";

	if (sales.length === 0) {
		empty.classList.remove("hidden");
		return;
	}
	empty.classList.add("hidden");

	sales.forEach(function (sale) {
		var li = document.createElement("li");
		li.className = "recent-sale-row" + (sale.voided ? " voided-row" : "");
		li.textContent =
			formatLocalTime(sale.created_at) +
			" — " +
			sale.quantity +
			" x " +
			sale.product_name +
			(sale.cups > 0 ? " + " + t("sell.recent_cups", { n: sale.cups }) : "") +
			" — " +
			formatMoney(sale.total) +
			(sale.voided ? " (" + t("common.voided_lower") + ")" : "");
		list.appendChild(li);
	});
}
