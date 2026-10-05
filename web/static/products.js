// Products page: search/filter, the product table, and the
// add/edit/adjust/archive actions.

var searchInput = document.getElementById("search-input");
var showArchivedInput = document.getElementById("show-archived-input");
var tbody = document.getElementById("products-tbody");
var emptyEl = document.getElementById("products-empty");
var emptyMessageEl = document.getElementById("products-empty-message");

var searchDebounce = null;

window.configReady.then(loadProducts);

document.getElementById("add-product-btn").onclick = openAddProductModal;
document.getElementById("add-product-btn-empty").onclick = openAddProductModal;

searchInput.addEventListener("input", function () {
	clearTimeout(searchDebounce);
	searchDebounce = setTimeout(loadProducts, 250);
});

showArchivedInput.addEventListener("change", loadProducts);

function loadProducts() {
	var params = new URLSearchParams();
	var q = searchInput.value.trim();
	if (q !== "") params.set("q", q);
	// "Show archived" off -> active only; on -> no filter, so both
	// active and archived products are shown.
	if (!showArchivedInput.checked) params.set("archived", "0");

	return apiFetch("/api/products?" + params.toString()).then(function (products) {
		renderProducts(products, q !== "");
	});
}

function renderProducts(products, isFiltered) {
	tbody.textContent = "";

	if (products.length === 0) {
		emptyMessageEl.textContent = isFiltered ? "No products match your search." : "No products yet.";
		emptyEl.classList.remove("hidden");
		return;
	}
	emptyEl.classList.add("hidden");

	products.forEach(function (p) {
		tbody.appendChild(buildProductRow(p));
	});
}

function buildProductRow(p) {
	var tr = document.createElement("tr");
	if (p.archived) tr.className = "archived-row";

	var nameTd = document.createElement("td");
	nameTd.textContent = p.name;
	tr.appendChild(nameTd);

	var skuTd = document.createElement("td");
	skuTd.textContent = p.sku || "—";
	tr.appendChild(skuTd);

	var priceTd = document.createElement("td");
	priceTd.textContent = formatMoney(p.price);
	tr.appendChild(priceTd);

	var stockTd = document.createElement("td");
	stockTd.textContent = p.stock;
	if (p.stock <= 0) {
		stockTd.classList.add("stock-red");
	} else if (p.stock <= p.low_stock_threshold) {
		stockTd.classList.add("stock-amber");
	}
	tr.appendChild(stockTd);

	var actionsTd = document.createElement("td");
	actionsTd.className = "row-actions";
	actionsTd.appendChild(makeActionButton("Edit", function () {
		openEditProductModal(p);
	}));
	actionsTd.appendChild(makeActionButton("Adjust", function () {
		openAdjustStockModal(p, loadProducts);
	}));
	actionsTd.appendChild(
		makeActionButton(p.archived ? "Unarchive" : "Archive", function () {
			setArchived(p, !p.archived);
		})
	);
	actionsTd.appendChild(makeActionButton("History", function () {
		window.location.href = "/stock-log?product=" + p.id;
	}));
	tr.appendChild(actionsTd);

	return tr;
}

function makeActionButton(label, onClick) {
	var btn = document.createElement("button");
	btn.type = "button";
	btn.className = "btn btn-secondary btn-small";
	btn.textContent = label;
	btn.onclick = onClick;
	return btn;
}

function setArchived(product, archived) {
	var action = archived ? "archive" : "unarchive";
	apiFetch("/api/products/" + product.id + "/" + action, { method: "POST", body: "{}" })
		.then(loadProducts)
		.catch(function (err) {
			console.error(err);
		});
}

function openAddProductModal() {
	openModal({
		title: "Add product",
		submitLabel: "Add",
		fields: [
			{ name: "name", label: "Name", autofocus: true },
			{ name: "sku", label: "SKU (optional)" },
			{ name: "price", label: "Price" },
			{ name: "stock", label: "Starting stock", value: "0" },
			{ name: "low_stock_threshold", label: "Low stock threshold", value: "5" },
		],
		onSubmit: function (values) {
			var payload = validateProductForm(values, { includeStock: true });
			return apiFetch("/api/products", { method: "POST", body: JSON.stringify(payload) }).then(loadProducts);
		},
	});
}

function openEditProductModal(product) {
	openModal({
		title: "Edit product",
		submitLabel: "Save",
		fields: [
			{ name: "name", label: "Name", value: product.name, autofocus: true },
			{ name: "sku", label: "SKU (optional)", value: product.sku },
			{ name: "price", label: "Price", value: formatPriceForInput(product.price) },
			{ name: "low_stock_threshold", label: "Low stock threshold", value: String(product.low_stock_threshold) },
		],
		onSubmit: function (values) {
			var payload = validateProductForm(values, { includeStock: false });
			return apiFetch("/api/products/" + product.id, { method: "PUT", body: JSON.stringify(payload) }).then(loadProducts);
		},
	});
}

// formatPriceForInput shows the stored price pre-filled in the edit
// form using the same integer-math formatter as display, just without
// the currency label.
function formatPriceForInput(cents) {
	var cfg = window.appConfig || { currency: "", decimals: 2 };
	var withCurrency = formatMoney(cents);
	return cfg.currency ? withCurrency.slice(0, withCurrency.length - cfg.currency.length - 1) : withCurrency;
}

// validateProductForm applies the same limits as the server (name
// 1-100 chars, sku <= 40 chars, whole-number price/stock/threshold in
// range) so the user sees inline errors before any request is sent.
function validateProductForm(values, opts) {
	var name = values.name.trim();
	if (name.length < 1 || name.length > 100) {
		throw fieldError("name", "Name must be 1-100 characters.");
	}

	var sku = values.sku.trim();
	if (sku.length > 40) {
		throw fieldError("sku", "SKU must be at most 40 characters.");
	}

	var price;
	try {
		price = parseMoney(values.price);
	} catch (err) {
		throw fieldError("price", err.message);
	}
	if (price < 0 || price > 1000000000) {
		throw fieldError("price", "Price must be between 0 and 1,000,000,000.");
	}

	var threshold;
	try {
		threshold = parseWholeNumber(values.low_stock_threshold, { message: "Enter a whole number." });
	} catch (err) {
		throw fieldError("low_stock_threshold", err.message);
	}
	if (threshold < 0 || threshold > 1000000) {
		throw fieldError("low_stock_threshold", "Threshold must be between 0 and 1,000,000.");
	}

	var payload = { name: name, sku: sku, price: price, low_stock_threshold: threshold };

	if (opts.includeStock) {
		var stock;
		try {
			stock = parseWholeNumber(values.stock, { message: "Enter a whole number." });
		} catch (err) {
			throw fieldError("stock", err.message);
		}
		if (stock < 0 || stock > 1000000) {
			throw fieldError("stock", "Stock must be between 0 and 1,000,000.");
		}
		payload.stock = stock;
	}

	return payload;
}
