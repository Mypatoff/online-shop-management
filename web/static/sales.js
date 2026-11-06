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
		renderTotals(data.totals, data.billiard, data.cups);
		renderByProduct(data.by_product, data.cups);
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

function renderTotals(totals, billiard, cups) {
	document.getElementById("day-revenue").textContent = formatMoney(totals.revenue);
	document.getElementById("day-units").textContent = totals.units;
	document.getElementById("day-count").textContent = totals.sale_count;
	document.getElementById("day-cups").textContent = cups.count;
	document.getElementById("day-billiard-revenue").textContent = formatMoney(billiard.revenue);
	document.getElementById("day-total-revenue-line").textContent = t("sales.total_line", {
		total: formatMoney(totals.total_revenue),
		shop: formatMoney(totals.revenue),
		billiard: formatMoney(billiard.revenue),
	});
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
		addCell(tr, entry.voided ? t("common.voided") : t("common.completed"));
		tbody.appendChild(tr);
	});
}

function renderByProduct(items, cups) {
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
	// Cups are sold with the products but belong to none of them: their own row, so the
	// column still adds up to the day's revenue.
	if (cups && cups.count > 0) {
		var cupsRow = document.createElement("tr");
		addCell(cupsRow, t("sales.col_cups"));
		addCell(cupsRow, cups.count);
		addCell(cupsRow, formatMoney(cups.revenue));
		tbody.appendChild(cupsRow);
	}
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
	addCell(tr, sale.cups > 0 ? sale.cups : "—");
	addCell(tr, formatMoney(sale.unit_price));
	addCell(tr, formatMoney(sale.total));
	addCell(tr, sale.voided ? t("common.voided") : t("common.completed"));

	var actionsTd = document.createElement("td");
	actionsTd.className = "row-actions";
	if (!sale.voided) {
		var voidBtn = document.createElement("button");
		voidBtn.type = "button";
		voidBtn.className = "btn btn-secondary btn-small";
		voidBtn.textContent = t("common.void");
		voidBtn.onclick = function () {
			confirmDialog(t("sales.void_confirm"), { title: t("sales.void_title"), confirmLabel: t("common.void") }).then(function (ok) {
				if (!ok) return;
				apiFetch("/api/sales/" + sale.id + "/void", { method: "POST", body: "{}" })
					.then(function () {
						loadDay(currentDate);
					})
					.catch(showError);
			});
		};
		actionsTd.appendChild(voidBtn);

		var receiptBtn = document.createElement("button");
		receiptBtn.type = "button";
		receiptBtn.className = "btn btn-secondary btn-small";
		receiptBtn.textContent = t("common.receipt");
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
	appendParagraph(container, t("report.title", { date: formatDateDisplay(data.date) }));
	appendParagraph(container, t("report.printed_at", { time: new Date().toLocaleString(localeTag()) }));

	var totalsList = document.createElement("ul");
	appendListItem(totalsList, t("report.revenue", { v: formatMoney(data.totals.revenue) }));
	appendListItem(totalsList, t("report.units_sold", { v: data.totals.units }));
	appendListItem(totalsList, t("report.sales", { v: data.totals.sale_count }));
	if (data.cups && data.cups.count > 0) {
		appendListItem(totalsList, t("report.cups", { n: data.cups.count, v: formatMoney(data.cups.revenue) }));
	}
	appendListItem(totalsList, t("report.billiard_revenue", { v: formatMoney(data.billiard.revenue) }));
	appendListItem(totalsList, t("report.total_revenue", { v: formatMoney(data.totals.total_revenue) }));
	container.appendChild(totalsList);

	if (data.totals.voided_count > 0) {
		appendParagraph(container, t("report.voided_sales", { n: data.totals.voided_count }));
	}
	var voidedBilliardCount = data.billiard.entries.filter(function (e) {
		return e.voided;
	}).length;
	if (voidedBilliardCount > 0) {
		appendParagraph(container, t("report.voided_billiard", { n: voidedBilliardCount }));
	}

	appendHeading(container, "h2", t("sales.by_product"));
	container.appendChild(
		buildReportTable(
			[t("sales.col_product"), t("sales.col_units"), t("sales.col_revenue")],
			data.by_product
				.map(function (item) {
					return [item.name, String(item.units), formatMoney(item.revenue)];
				})
				.concat(data.cups && data.cups.count > 0 ? [[t("sales.col_cups"), String(data.cups.count), formatMoney(data.cups.revenue)]] : [])
		)
	);

	appendHeading(container, "h2", t("sales.sales"));
	container.appendChild(
		buildReportTable(
			[t("common.time"), t("sales.col_product"), t("sales.col_qty"), t("sales.col_cups"), t("sales.col_unit_price"), t("sales.col_total"), t("common.status")],
			data.sales.map(function (sale) {
				return [
					formatTimeOfDay(sale.created_at),
					sale.product_name,
					String(sale.quantity),
					sale.cups > 0 ? String(sale.cups) : "—",
					formatMoney(sale.unit_price),
					formatMoney(sale.total),
					sale.voided ? t("common.voided") : t("common.completed"),
				];
			})
		)
	);

	appendHeading(container, "h2", t("nav.billiard"));
	container.appendChild(
		buildReportTable(
			[t("common.time"), t("billiard.col_table"), t("billiard.col_minutes"), t("billiard.col_note"), t("billiard.col_amount"), t("common.status")],
			data.billiard.entries.map(function (entry) {
				return [
					formatTimeOfDay(entry.created_at),
					entry.table || "—",
					entry.minutes != null ? String(entry.minutes) : "—",
					entry.note || "—",
					formatMoney(entry.amount),
					entry.voided ? t("common.voided") : t("common.completed"),
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
	appendParagraph(container, formatLocalTime(sale.created_at));
	appendParagraph(container, sale.product_name);
	appendParagraph(container, t("receipt.qty", { v: sale.quantity }));
	appendParagraph(container, t("receipt.unit_price", { v: formatMoney(sale.unit_price) }));
	if (sale.cups > 0) {
		appendParagraph(container, t("receipt.cups", { n: sale.cups, price: formatMoney(sale.cup_price), v: formatMoney(sale.cups * sale.cup_price) }));
	}
	appendParagraph(container, t("receipt.total", { v: formatMoney(sale.total) }));
	appendParagraph(container, t("receipt.thanks"));

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
