// Sell page: search an active product, pick a quantity, and sell it.

var allProducts = [];
var selected = null;

var searchInput = document.getElementById("sell-search");
var resultsList = document.getElementById("sell-results");
var selectedPanel = document.getElementById("sell-selected");
var selectedNameEl = document.getElementById("selected-name");
var selectedPriceEl = document.getElementById("selected-price");
var qtyInput = document.getElementById("qty-input");
var totalEl = document.getElementById("sell-total");
var sellBtn = document.getElementById("sell-btn");
var sellErrorEl = document.getElementById("sell-error");

window.configReady.then(function () {
	loadProducts();
	loadRecentSales();
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

sellBtn.onclick = function () {
	if (!selected) return;
	var quantity = currentQuantity();
	sellErrorEl.textContent = "";
	sellBtn.disabled = true;

	apiFetch("/api/sales", {
		method: "POST",
		body: JSON.stringify({ product_id: selected.id, quantity: quantity }),
	})
		.then(function (res) {
			showToast("Sold " + quantity + " x " + selected.name + ", " + res.stock + " left");
			updateProductStock(selected.id, res.stock);
			selectProduct(selected);
			loadRecentSales();
		})
		.catch(function (err) {
			sellErrorEl.textContent = err.message;
		})
		.finally(function () {
			sellBtn.disabled = false;
		});
};

function loadProducts() {
	return apiFetch("/api/products?archived=0").then(function (products) {
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
		btn.textContent = p.name + (p.sku ? " (" + p.sku + ")" : "") + " — " + formatMoney(p.price) + " — " + p.stock + " in stock";
		btn.onclick = function () {
			selectProduct(p);
		};
		li.appendChild(btn);
		resultsList.appendChild(li);
	});
}

function selectProduct(p) {
	// Use the freshest copy in allProducts (stock may have just changed).
	selected = allProducts.find(function (item) {
		return item.id === p.id;
	}) || p;

	selectedPanel.classList.remove("hidden");
	selectedNameEl.textContent = selected.name;
	selectedPriceEl.textContent = formatMoney(selected.price) + " each";
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

function updateTotal() {
	if (!selected) {
		totalEl.textContent = "—";
		return;
	}
	totalEl.textContent = formatMoney(selected.price * currentQuantity());
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
			formatLocalTime(sale.created_at) + " — " + sale.quantity + " x " + sale.product_name + " — " + formatMoney(sale.total) + (sale.voided ? " (voided)" : "");
		list.appendChild(li);
	});
}
