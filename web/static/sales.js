// Sales page: a date-navigable daily history (totals, per-product
// breakdown, full sale list with void), plus printing a daily report
// or a single-sale receipt.

var currentDate = dateFromQueryParam() || formatDateInput(new Date());
var currentData = null; // last response from /api/sales/daily, used for printing

// dateFromQueryParam lets links like the dashboard chart's bars
// (/sales?date=YYYY-MM-DD) preselect a day.
function dateFromQueryParam() {
	var v = new URLSearchParams(window.location.search).get("date");
	return v && /^\d{4}-\d{2}-\d{2}$/.test(v) ? v : null;
}

var dateInput = document.getElementById("date-input");
var prevBtn = document.getElementById("prev-day-btn");
var nextBtn = document.getElementById("next-day-btn");
var printReportBtn = document.getElementById("print-report-btn");

dateInput.value = currentDate;

window.configReady.then(function () {
	loadDay(currentDate);
});

dateInput.addEventListener("change", function () {
	if (dateInput.value) loadDay(dateInput.value);
});

prevBtn.addEventListener("click", function () {
	loadDay(shiftDate(currentDate, -1));
});

nextBtn.addEventListener("click", function () {
	loadDay(shiftDate(currentDate, 1));
});

printReportBtn.addEventListener("click", function () {
	if (currentData) buildDailyReport(currentData);
	window.print();
});

function loadDay(dateStr) {
	currentDate = dateStr;
	dateInput.value = dateStr;
	updateNextDisabled();

	apiFetch("/api/sales/daily?date=" + encodeURIComponent(dateStr)).then(function (data) {
		currentData = data;
		renderTotals(data.totals, data.billiard);
		renderByProduct(data.by_product);
		renderSales(data.sales);
		renderBilliardDay(data.billiard);
	});
}

function updateNextDisabled() {
	nextBtn.disabled = currentDate >= formatDateInput(new Date());
}

function shiftDate(dateStr, deltaDays) {
	var parts = dateStr.split("-");
	var d = new Date(Number(parts[0]), Number(parts[1]) - 1, Number(parts[2]));
	d.setDate(d.getDate() + deltaDays);
	return formatDateInput(d);
}

// formatDateInput formats a Date in local time as YYYY-MM-DD, matching
// what <input type="date"> and the API's ?date= parameter expect.
function formatDateInput(date) {
	var y = date.getFullYear();
	var m = String(date.getMonth() + 1).padStart(2, "0");
	var d = String(date.getDate()).padStart(2, "0");
	return y + "-" + m + "-" + d;
}

// formatTimeOfDay returns just HH:MM, local time: the day is already
// fixed by the page, so repeating it per row would be noise.
function formatTimeOfDay(unixSeconds) {
	var d = new Date(unixSeconds * 1000);
	var h = String(d.getHours()).padStart(2, "0");
	var m = String(d.getMinutes()).padStart(2, "0");
	return h + ":" + m;
}

function renderTotals(totals, billiard) {
	document.getElementById("day-revenue").textContent = formatMoney(totals.revenue);
	document.getElementById("day-units").textContent = totals.units;
	document.getElementById("day-count").textContent = totals.sale_count;
	document.getElementById("day-billiard-revenue").textContent = formatMoney(billiard.revenue);
	document.getElementById("day-total-revenue-line").textContent =
		"Total revenue: " + formatMoney(totals.total_revenue) + " (shop " + formatMoney(totals.revenue) + " + billiard " + formatMoney(billiard.revenue) + ")";
}

function renderBilliardDay(billiard) {
	var tbody = document.getElementById("billiard-day-tbody");
	var empty = document.getElementById("billiard-day-empty");
	var wrap = document.getElementById("billiard-day-wrap");
	tbody.textContent = "";

	var entries = (billiard && billiard.entries) || [];
	if (entries.length === 0) {
		empty.classList.remove("hidden");
		wrap.classList.add("hidden");
		return;
	}
	empty.classList.add("hidden");
	wrap.classList.remove("hidden");

	entries.forEach(function (entry) {
		var tr = document.createElement("tr");
		if (entry.voided) tr.className = "voided-row";
		addCell(tr, formatTimeOfDay(entry.created_at));
		addCell(tr, entry.table || "—");
		addCell(tr, entry.minutes != null ? String(entry.minutes) : "—");
		addCell(tr, entry.note || "—");
		addCell(tr, formatMoney(entry.amount));
		addCell(tr, entry.voided ? "Voided" : "Completed");
		tbody.appendChild(tr);
	});
}

function renderByProduct(items) {
	var tbody = document.getElementById("by-product-tbody");
	var empty = document.getElementById("by-product-empty");
	var wrap = document.getElementById("by-product-wrap");
	tbody.textContent = "";

	if (!items || items.length === 0) {
		empty.classList.remove("hidden");
		wrap.classList.add("hidden");
		return;
	}
	empty.classList.add("hidden");
	wrap.classList.remove("hidden");

	items.forEach(function (item) {
		var tr = document.createElement("tr");
		addCell(tr, item.name);
		addCell(tr, item.units);
		addCell(tr, formatMoney(item.revenue));
		tbody.appendChild(tr);
	});
}

function renderSales(sales) {
	var tbody = document.getElementById("sales-tbody");
	var empty = document.getElementById("sales-empty");
	var wrap = document.getElementById("sales-wrap");
	tbody.textContent = "";

	if (!sales || sales.length === 0) {
		empty.classList.remove("hidden");
		wrap.classList.add("hidden");
		return;
	}
	empty.classList.add("hidden");
	wrap.classList.remove("hidden");

	sales.forEach(function (sale) {
		tbody.appendChild(buildSaleRow(sale));
	});
}

function buildSaleRow(sale) {
	var tr = document.createElement("tr");
	if (sale.voided) tr.className = "voided-row";

	addCell(tr, formatTimeOfDay(sale.created_at));
	addCell(tr, sale.product_name);
	addCell(tr, sale.quantity);
	addCell(tr, formatMoney(sale.unit_price));
	addCell(tr, formatMoney(sale.total));
	addCell(tr, sale.voided ? "Voided" : "Completed");

	var actionsTd = document.createElement("td");
	actionsTd.className = "row-actions";
	if (!sale.voided) {
		var voidBtn = document.createElement("button");
		voidBtn.type = "button";
		voidBtn.className = "btn btn-secondary btn-small";
		voidBtn.textContent = "Void";
		voidBtn.onclick = function () {
			if (!confirm("Void this sale? This restores the stock it used.")) return;
			apiFetch("/api/sales/" + sale.id + "/void", { method: "POST", body: "{}" })
				.then(function () {
					loadDay(currentDate);
				})
				.catch(function (err) {
					alert(err.message);
				});
		};
		actionsTd.appendChild(voidBtn);

		var receiptBtn = document.createElement("button");
		receiptBtn.type = "button";
		receiptBtn.className = "btn btn-secondary btn-small";
		receiptBtn.textContent = "Receipt";
		receiptBtn.onclick = function () {
			printReceipt(sale);
		};
		actionsTd.appendChild(receiptBtn);
	}
	tr.appendChild(actionsTd);

	return tr;
}

function addCell(tr, text) {
	var td = document.createElement("td");
	td.textContent = text;
	tr.appendChild(td);
}

// buildDailyReport fills the print-only #daily-report block from the
// same data already on screen, so "Print daily report" always prints
// exactly what's displayed.
function buildDailyReport(data) {
	var container = document.getElementById("daily-report");
	container.textContent = "";
	document.getElementById("receipt").textContent = "";

	var shopName = (window.appConfig && window.appConfig.shop_name) || "ShopKeeper";

	appendHeading(container, "h1", shopName);
	appendParagraph(container, "Daily sales report — " + data.date);
	appendParagraph(container, "Printed at " + new Date().toLocaleString());

	var totalsList = document.createElement("ul");
	appendListItem(totalsList, "Revenue: " + formatMoney(data.totals.revenue));
	appendListItem(totalsList, "Units sold: " + data.totals.units);
	appendListItem(totalsList, "Sales: " + data.totals.sale_count);
	appendListItem(totalsList, "Billiard revenue: " + formatMoney(data.billiard.revenue));
	appendListItem(totalsList, "Total revenue: shop + billiard = " + formatMoney(data.totals.total_revenue));
	container.appendChild(totalsList);

	if (data.totals.voided_count > 0) {
		appendParagraph(container, data.totals.voided_count + " voided sale" + (data.totals.voided_count === 1 ? "" : "s") + " not counted.");
	}
	var voidedBilliardCount = data.billiard.entries.filter(function (e) {
		return e.voided;
	}).length;
	if (voidedBilliardCount > 0) {
		appendParagraph(container, voidedBilliardCount + " voided billiard entr" + (voidedBilliardCount === 1 ? "y" : "ies") + " not counted.");
	}

	appendHeading(container, "h2", "By product");
	container.appendChild(
		buildReportTable(
			["Product", "Units", "Revenue"],
			data.by_product.map(function (item) {
				return [item.name, String(item.units), formatMoney(item.revenue)];
			})
		)
	);

	appendHeading(container, "h2", "Sales");
	container.appendChild(
		buildReportTable(
			["Time", "Product", "Qty", "Unit price", "Total", "Status"],
			data.sales.map(function (sale) {
				return [
					formatTimeOfDay(sale.created_at),
					sale.product_name,
					String(sale.quantity),
					formatMoney(sale.unit_price),
					formatMoney(sale.total),
					sale.voided ? "Voided" : "Completed",
				];
			})
		)
	);

	appendHeading(container, "h2", "Billiard");
	container.appendChild(
		buildReportTable(
			["Time", "Table", "Minutes", "Note", "Amount", "Status"],
			data.billiard.entries.map(function (entry) {
				return [
					formatTimeOfDay(entry.created_at),
					entry.table || "—",
					entry.minutes != null ? String(entry.minutes) : "—",
					entry.note || "—",
					formatMoney(entry.amount),
					entry.voided ? "Voided" : "Completed",
				];
			})
		)
	);
}

function appendHeading(container, tag, text) {
	var el = document.createElement(tag);
	el.textContent = text;
	container.appendChild(el);
}

function appendParagraph(container, text) {
	var p = document.createElement("p");
	p.textContent = text;
	container.appendChild(p);
}

function appendListItem(list, text) {
	var li = document.createElement("li");
	li.textContent = text;
	list.appendChild(li);
}

function buildReportTable(headers, rows) {
	var table = document.createElement("table");
	table.className = "data-table";

	var thead = document.createElement("thead");
	var headRow = document.createElement("tr");
	headers.forEach(function (text) {
		var th = document.createElement("th");
		th.textContent = text;
		headRow.appendChild(th);
	});
	thead.appendChild(headRow);
	table.appendChild(thead);

	var tbody = document.createElement("tbody");
	rows.forEach(function (cells) {
		var tr = document.createElement("tr");
		cells.forEach(function (text) {
			var td = document.createElement("td");
			td.textContent = text;
			tr.appendChild(td);
		});
		tbody.appendChild(tr);
	});
	table.appendChild(tbody);

	return table;
}

// printReceipt fills the print-only #receipt block for one sale, adds
// a narrow @page size so it prints like a till receipt, prints, and
// removes that page style again once printing is done.
function printReceipt(sale) {
	var container = document.getElementById("receipt");
	container.textContent = "";
	container.className = "print-only receipt";
	document.getElementById("daily-report").textContent = "";

	var shopName = (window.appConfig && window.appConfig.shop_name) || "ShopKeeper";

	appendParagraph(container, shopName);
	appendParagraph(container, new Date(sale.created_at * 1000).toLocaleString());
	appendParagraph(container, sale.product_name);
	appendParagraph(container, "Qty: " + sale.quantity);
	appendParagraph(container, "Unit price: " + formatMoney(sale.unit_price));
	appendParagraph(container, "Total: " + formatMoney(sale.total));
	appendParagraph(container, "Thank you");

	var style = document.createElement("style");
	style.id = "receipt-page";
	style.textContent = "@page{size:80mm auto;margin:4mm}";
	document.head.appendChild(style);

	function cleanup() {
		var el = document.getElementById("receipt-page");
		if (el) el.remove();
		window.removeEventListener("afterprint", cleanup);
	}
	window.addEventListener("afterprint", cleanup);

	window.print();
}
