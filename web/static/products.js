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
		emptyMessageEl.textContent = isFiltered ? t("products.no_match") : t("products.empty");
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
	actionsTd.appendChild(makeActionButton(t("common.edit"), function () {
		openEditProductModal(p);
	}));
	actionsTd.appendChild(makeActionButton(t("products.adjust"), function () {
		openAdjustStockModal(p, loadProducts);
	}));
	actionsTd.appendChild(
		makeActionButton(p.archived ? t("products.unarchive") : t("products.archive"), function () {
			setArchived(p, !p.archived);
		})
	);
	actionsTd.appendChild(makeActionButton(t("products.history"), function () {
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
		.catch(showError);
}

function openAddProductModal() {
	openModal({
		title: t("products.add"),
		submitLabel: t("common.add"),
		fields: [
			{ name: "name", label: t("products.f_name"), autofocus: true },
			{ name: "sku", label: t("products.f_sku") },
			{ name: "price", label: t("products.f_price") },
			{ name: "stock", label: t("products.f_stock"), value: "0" },
			{ name: "low_stock_threshold", label: t("products.f_threshold"), value: "5" },
		],
		onSubmit: function (values) {
			var payload = validateProductForm(values, { includeStock: true });
			return apiFetch("/api/products", { method: "POST", body: JSON.stringify(payload) }).then(loadProducts);
		},
	});
}

function openEditProductModal(product) {
	openModal({
		title: t("products.edit"),
		submitLabel: t("common.save"),
		fields: [
			{ name: "name", label: t("products.f_name"), value: product.name, autofocus: true },
			{ name: "sku", label: t("products.f_sku"), value: product.sku },
			{ name: "price", label: t("products.f_price"), value: formatPriceForInput(product.price) },
			{ name: "low_stock_threshold", label: t("products.f_threshold"), value: String(product.low_stock_threshold) },
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
		throw fieldError("name", t("products.err_name"));
	}

	var sku = values.sku.trim();
	if (sku.length > 40) {
		throw fieldError("sku", t("products.err_sku"));
	}

	var price;
	try {
		price = parseMoney(values.price);
	} catch (err) {
		throw fieldError("price", err.message);
	}
	if (price < 0 || price > 100000000000) {
		throw fieldError("price", t("products.err_price"));
	}

	var threshold;
	try {
		threshold = parseWholeNumber(values.low_stock_threshold, { message: t("number.whole") });
	} catch (err) {
		throw fieldError("low_stock_threshold", err.message);
	}
	if (threshold < 0 || threshold > 1000000) {
		throw fieldError("low_stock_threshold", t("products.err_threshold"));
	}

	var payload = { name: name, sku: sku, price: price, low_stock_threshold: threshold };

	if (opts.includeStock) {
		var stock;
		try {
			stock = parseWholeNumber(values.stock, { message: t("number.whole") });
		} catch (err) {
			throw fieldError("stock", err.message);
		}
		if (stock < 0 || stock > 1000000) {
			throw fieldError("stock", t("products.err_stock"));
		}
		payload.stock = stock;
	}

	return payload;
}
