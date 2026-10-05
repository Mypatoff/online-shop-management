// Stock log page: a product filter and a paged table of every stock
// movement (initial/sale/void/adjust), newest first.

var productFilter = document.getElementById("product-filter");
var tbody = document.getElementById("stock-log-tbody");
var emptyEl = document.getElementById("stock-log-empty");
var wrapEl = document.getElementById("stock-log-wrap");
var loadMoreBtn = document.getElementById("load-more-btn");

var nextBeforeID = null;

window.configReady.then(function () {
	return apiFetch("/api/products").then(function (products) {
		populateProductFilter(products);
	});
}).then(function () {
	loadLog(true);
});

productFilter.addEventListener("change", function () {
	loadLog(true);
});

loadMoreBtn.addEventListener("click", function () {
	loadLog(false);
});

function populateProductFilter(products) {
	var allOption = document.createElement("option");
	allOption.value = "";
	allOption.textContent = "All products";
	productFilter.appendChild(allOption);

	products.forEach(function (p) {
		var option = document.createElement("option");
		option.value = String(p.id);
		option.textContent = p.name;
		productFilter.appendChild(option);
	});

	var preselect = new URLSearchParams(window.location.search).get("product");
	if (preselect && products.some(function (p) { return String(p.id) === preselect; })) {
		productFilter.value = preselect;
	}
}

function loadLog(reset) {
	if (reset) {
		tbody.textContent = "";
		nextBeforeID = null;
	}

	var params = new URLSearchParams();
	if (productFilter.value) params.set("product_id", productFilter.value);
	params.set("limit", "100");
	if (!reset && nextBeforeID != null) params.set("before_id", String(nextBeforeID));

	return apiFetch("/api/stock-log?" + params.toString()).then(function (res) {
		appendMovements(res.movements);
		nextBeforeID = res.next_before_id;
		loadMoreBtn.classList.toggle("hidden", nextBeforeID == null);
		updateEmptyState();
	});
}

function updateEmptyState() {
	var hasRows = tbody.children.length > 0;
	emptyEl.classList.toggle("hidden", hasRows);
	wrapEl.classList.toggle("hidden", !hasRows);
}

function appendMovements(movements) {
	(movements || []).forEach(function (mv) {
		tbody.appendChild(buildMovementRow(mv));
	});
}

function buildMovementRow(mv) {
	var tr = document.createElement("tr");

	addCell(tr, formatLocalTime(mv.created_at));
	addCell(tr, mv.product_name);
	addCell(tr, capitalize(mv.type));

	var changeTd = document.createElement("td");
	changeTd.textContent = (mv.delta > 0 ? "+" : "") + mv.delta;
	changeTd.classList.add(mv.delta >= 0 ? "delta-positive" : "delta-negative");
	tr.appendChild(changeTd);

	addCell(tr, mv.stock_after);
	addCell(tr, mv.reason || "—");
	addCell(tr, mv.note || "—");

	return tr;
}

function addCell(tr, text) {
	var td = document.createElement("td");
	td.textContent = text;
	tr.appendChild(td);
}

function capitalize(s) {
	return s.length ? s.charAt(0).toUpperCase() + s.slice(1) : s;
}
