// Dashboard page: stat cards and the low-stock list, both loaded from
// /api/summary.

window.configReady.then(loadSummary).catch(showPageError);

function loadSummary() {
	return apiFetch("/api/summary").then(renderSummary).catch(showPageError);
}

function renderSummary(summary) {
	document.getElementById("stat-products").textContent = summary.active_products;
	document.getElementById("stat-units").textContent = summary.units_in_stock;
	document.getElementById("stat-stock-value").textContent = formatMoney(summary.stock_value);
	document.getElementById("stat-revenue").textContent = formatMoney(summary.total_revenue_today);
	document.getElementById("stat-revenue-breakdown").textContent = "Shop " + formatMoney(summary.today_revenue) + " + Billiard " + formatMoney(summary.billiard_revenue_today);
	document.getElementById("stat-sales").textContent = summary.today_sale_count;
	document.getElementById("stat-billiard").textContent = summary.billiard_count_today;

	renderLowStock(summary.low_stock || []);
}

function renderLowStock(items) {
	var list = document.getElementById("low-stock-list");
	var empty = document.getElementById("low-stock-empty");
	list.textContent = "";

	if (items.length === 0) {
		empty.classList.remove("hidden");
		return;
	}
	empty.classList.add("hidden");

	items.forEach(function (item) {
		var li = document.createElement("li");
		li.className = "low-stock-row";

		var info = document.createElement("span");
		info.textContent = item.name + " — " + item.stock + " left (threshold " + item.low_stock_threshold + ")";
		li.appendChild(info);

		var btn = document.createElement("button");
		btn.type = "button";
		btn.className = "btn btn-secondary btn-small";
		btn.textContent = "Adjust";
		btn.onclick = function () {
			openAdjustStockModal(item, function () {
				loadSummary();
			});
		};
		li.appendChild(btn);

		list.appendChild(li);
	});
}

function showPageError(err) {
	// There's no dedicated error banner on the dashboard; the stat
	// cards simply keep their "—" placeholder if this fails.
	console.error(err);
}

// ---- daily sales chart ----
//
// An inline SVG bar chart built with createElementNS, scaling to any
// width via viewBox. Range (7/30 days) and metric (revenue/units)
// toggles aren't remembered across visits; they always start at 7
// days / Revenue.

var SVG_NS = "http://www.w3.org/2000/svg";
var chartState = { range: 7, metric: "revenue", data: null };
var chartResizeTimer = null;
var chartActiveTouchBar = null;

window.configReady.then(loadChart).catch(showChartError);

document.getElementById("chart-range-toggle").addEventListener("click", function (event) {
	var btn = event.target.closest(".segmented-btn");
	if (!btn || btn.classList.contains("active")) return;
	setActiveSegment(event.currentTarget, btn);
	chartState.range = Number(btn.dataset.value);
	loadChart();
});

document.getElementById("chart-metric-toggle").addEventListener("click", function (event) {
	var btn = event.target.closest(".segmented-btn");
	if (!btn || btn.classList.contains("active")) return;
	setActiveSegment(event.currentTarget, btn);
	chartState.metric = btn.dataset.value;
	renderChart();
});

window.addEventListener("resize", function () {
	clearTimeout(chartResizeTimer);
	chartResizeTimer = setTimeout(function () {
		if (chartState.data) renderChart();
	}, 150);
});

function setActiveSegment(group, btn) {
	group.querySelectorAll(".segmented-btn").forEach(function (b) {
		b.classList.remove("active");
		b.setAttribute("aria-pressed", "false");
	});
	btn.classList.add("active");
	btn.setAttribute("aria-pressed", "true");
}

function loadChart() {
	var status = document.getElementById("chart-status");
	var container = document.getElementById("chart-container");
	status.textContent = "Loading chart...";
	status.classList.remove("hidden");
	container.classList.add("hidden");

	return apiFetch("/api/sales/chart?days=" + chartState.range)
		.then(function (data) {
			chartState.data = data;
			status.classList.add("hidden");
			container.classList.remove("hidden");
			renderChart();
		})
		.catch(showChartError);
}

function showChartError(err) {
	var status = document.getElementById("chart-status");
	status.textContent = "Could not load chart: " + (err && err.message ? err.message : "unknown error");
	status.classList.remove("hidden");
	document.getElementById("chart-container").classList.add("hidden");
	console.error(err);
}

function renderChart() {
	var data = chartState.data;
	var container = document.getElementById("chart-container");
	container.textContent = "";
	if (!data || data.length === 0) return;

	var metric = chartState.metric;
	var values = data.map(function (d) {
		return metric === "units" ? d.units : d.revenue + d.billiard_revenue;
	});
	var maxValue = Math.max.apply(null, values);
	var allZero = maxValue === 0;
	var axisMax = niceCeil(allZero ? (metric === "units" ? 10 : 1000) : maxValue);

	var width = 600;
	var height = 220;
	var marginLeft = 78;
	var marginRight = 10;
	var marginTop = 10;
	var marginBottom = 28;
	var plotWidth = width - marginLeft - marginRight;
	var plotHeight = height - marginTop - marginBottom;

	var svg = document.createElementNS(SVG_NS, "svg");
	svg.setAttribute("viewBox", "0 0 " + width + " " + height);
	svg.setAttribute("class", "chart-svg");
	svg.setAttribute("role", "img");
	svg.setAttribute("aria-label", buildAriaSummary(data, metric));

	for (var i = 1; i <= 4; i++) {
		var frac = i / 4;
		var y = marginTop + plotHeight * (1 - frac);

		var line = document.createElementNS(SVG_NS, "line");
		line.setAttribute("x1", marginLeft);
		line.setAttribute("x2", width - marginRight);
		line.setAttribute("y1", y);
		line.setAttribute("y2", y);
		line.setAttribute("class", "chart-gridline");
		svg.appendChild(line);

		var tickValue = Math.round(axisMax * frac);
		var label = document.createElementNS(SVG_NS, "text");
		label.setAttribute("x", marginLeft - 8);
		label.setAttribute("y", y);
		label.setAttribute("text-anchor", "end");
		label.setAttribute("dominant-baseline", "middle");
		label.setAttribute("class", "chart-axis-label");
		label.textContent = metric === "units" ? String(tickValue) : formatMoney(tickValue);
		svg.appendChild(label);
	}

	var baseline = document.createElementNS(SVG_NS, "line");
	baseline.setAttribute("x1", marginLeft);
	baseline.setAttribute("x2", width - marginRight);
	baseline.setAttribute("y1", marginTop + plotHeight);
	baseline.setAttribute("y2", marginTop + plotHeight);
	baseline.setAttribute("class", "chart-axis-line");
	svg.appendChild(baseline);

	var n = data.length;
	var barSlot = plotWidth / n;
	var barWidth = Math.max(barSlot * 0.55, 2);
	var todayIndex = n - 1;
	var showEveryLabel = chartState.range !== 30;
	var barsToGrow = [];
	var baselineY = marginTop + plotHeight;

	// In the revenue view each day stacks two segments (shop, then
	// billiard on top); the units view stays shop-only, a single
	// segment. Both are drawn as "fill" rects that grow from zero
	// height, under one invisible "hit" rect sized to the whole stack
	// that carries the actual interactivity (focus/hover/tap/click) -
	// so a bar with two colors is still one tab stop and one target.
	data.forEach(function (entry, i) {
		var isToday = i === todayIndex;
		var x = marginLeft + i * barSlot + (barSlot - barWidth) / 2;
		var segments =
			metric === "units"
				? [{ value: entry.units, cls: "chart-bar-shop" }]
				: [
						{ value: entry.revenue, cls: "chart-bar-shop" },
						{ value: entry.billiard_revenue, cls: "chart-bar-billiard" },
					];
		var total = segments.reduce(function (sum, s) {
			return sum + s.value;
		}, 0);

		var stackTopY, stackHeight;
		if (total === 0) {
			stackHeight = 1.5;
			stackTopY = baselineY - stackHeight;
			var stub = document.createElementNS(SVG_NS, "rect");
			stub.setAttribute("x", x);
			stub.setAttribute("width", barWidth);
			stub.setAttribute("rx", "2");
			stub.setAttribute("class", "chart-bar-fill chart-bar-zero");
			stub.setAttribute("aria-hidden", "true");
			svg.appendChild(stub);
			stub.setAttribute("y", baselineY);
			stub.setAttribute("height", 0);
			barsToGrow.push({ rect: stub, y: stackTopY, height: stackHeight });
		} else {
			var cumulative = 0;
			segments.forEach(function (seg) {
				if (seg.value <= 0) return;
				var segHeight = Math.max((seg.value / axisMax) * plotHeight, 1);
				var yTop = baselineY - cumulative - segHeight;
				var rect = document.createElementNS(SVG_NS, "rect");
				rect.setAttribute("x", x);
				rect.setAttribute("width", barWidth);
				rect.setAttribute("class", "chart-bar-fill " + seg.cls);
				rect.setAttribute("aria-hidden", "true");
				svg.appendChild(rect);
				rect.setAttribute("y", baselineY);
				rect.setAttribute("height", 0);
				barsToGrow.push({ rect: rect, y: yTop, height: segHeight });
				cumulative += segHeight;
			});
			stackHeight = cumulative;
			stackTopY = baselineY - stackHeight;
		}

		var hit = document.createElementNS(SVG_NS, "rect");
		hit.setAttribute("x", x);
		hit.setAttribute("width", barWidth);
		hit.setAttribute("y", stackTopY);
		hit.setAttribute("height", Math.max(stackHeight, 1));
		hit.setAttribute("rx", "2");
		hit.setAttribute("tabindex", "0");
		hit.setAttribute("role", "button");
		hit.setAttribute("pointer-events", "all");
		hit.setAttribute("aria-label", barAriaLabel(entry, metric));
		hit.setAttribute("class", "chart-bar chart-bar-hit" + (isToday ? " chart-bar-today" : ""));
		wireBarInteractivity(hit, entry);
		svg.appendChild(hit);

		if (showEveryLabel || i % 5 === 0) {
			var xLabel = document.createElementNS(SVG_NS, "text");
			xLabel.setAttribute("x", x + barWidth / 2);
			xLabel.setAttribute("y", height - 8);
			xLabel.setAttribute("text-anchor", "middle");
			xLabel.setAttribute("class", "chart-axis-label");
			xLabel.textContent = shortDateLabel(entry.date);
			svg.appendChild(xLabel);
		}
	});

	if (allZero) {
		var emptyLabel = document.createElementNS(SVG_NS, "text");
		emptyLabel.setAttribute("x", marginLeft + plotWidth / 2);
		emptyLabel.setAttribute("y", marginTop + plotHeight / 2);
		emptyLabel.setAttribute("text-anchor", "middle");
		emptyLabel.setAttribute("class", "chart-empty-label");
		emptyLabel.textContent = "No sales yet in this period";
		svg.appendChild(emptyLabel);
	}

	container.appendChild(svg);
	container.appendChild(buildTooltipEl());
	container.appendChild(buildFallbackTable(data));

	// Grow the bars from zero height. Reading a layout property first
	// forces the browser to commit the y=0/height=0 starting state
	// before the final values are set, so the CSS transition (which
	// prefers-reduced-motion disables) actually has something to
	// animate from.
	requestAnimationFrame(function () {
		svg.getBoundingClientRect();
		requestAnimationFrame(function () {
			barsToGrow.forEach(function (b) {
				b.rect.setAttribute("y", b.y);
				b.rect.setAttribute("height", b.height);
			});
		});
	});
}

// niceCeil rounds value up to a "nice" number (1/2/5/10 times a power
// of ten) so Y-axis gridlines land on round values.
function niceCeil(value) {
	if (value <= 0) return 1;
	var exponent = Math.floor(Math.log(value) / Math.LN10);
	var fraction = value / Math.pow(10, exponent);
	var niceFraction;
	if (fraction <= 1) niceFraction = 1;
	else if (fraction <= 2) niceFraction = 2;
	else if (fraction <= 5) niceFraction = 5;
	else niceFraction = 10;
	return niceFraction * Math.pow(10, exponent);
}

function parseLocalDate(dateStr) {
	var parts = dateStr.split("-");
	return new Date(Number(parts[0]), Number(parts[1]) - 1, Number(parts[2]));
}

function shortDateLabel(dateStr) {
	var d = parseLocalDate(dateStr);
	return d.toLocaleDateString(undefined, { weekday: "short" }) + " " + d.getDate();
}

function fullDateLabel(dateStr) {
	var d = parseLocalDate(dateStr);
	return d.toLocaleDateString(undefined, { weekday: "long", year: "numeric", month: "long", day: "numeric" });
}

function barAriaLabel(entry, metric) {
	if (metric === "units") {
		return (
			fullDateLabel(entry.date) + ", " + entry.units + " units, " + entry.sale_count + " sale" + (entry.sale_count === 1 ? "" : "s") + ". Opens that day on the Sales page."
		);
	}
	var total = entry.revenue + entry.billiard_revenue;
	return (
		fullDateLabel(entry.date) +
		", total " +
		formatMoney(total) +
		" (shop " +
		formatMoney(entry.revenue) +
		", billiard " +
		formatMoney(entry.billiard_revenue) +
		"). Opens that day on the Sales page."
	);
}

function buildAriaSummary(data, metric) {
	if (metric === "units") {
		var totalUnits = 0;
		var bestIndex = 0;
		var bestUnits = -1;
		data.forEach(function (entry, i) {
			totalUnits += entry.units;
			if (entry.units > bestUnits) {
				bestUnits = entry.units;
				bestIndex = i;
			}
		});
		var unitsDetail = bestUnits > 0 ? "best day was " + fullDateLabel(data[bestIndex].date) + " with " + data[bestIndex].units + " units" : "no sales in this period";
		return "Daily sales chart showing units sold for the last " + data.length + " days. Total " + totalUnits + " units; " + unitsDetail + ".";
	}

	var totalShop = 0;
	var totalBilliard = 0;
	var bestIndex2 = 0;
	var bestCombined = -1;
	data.forEach(function (entry, i) {
		totalShop += entry.revenue;
		totalBilliard += entry.billiard_revenue;
		var combined = entry.revenue + entry.billiard_revenue;
		if (combined > bestCombined) {
			bestCombined = combined;
			bestIndex2 = i;
		}
	});
	var best = data[bestIndex2];
	var detail =
		bestCombined > 0
			? "best day was " + fullDateLabel(best.date) + " with " + formatMoney(bestCombined) + " (shop " + formatMoney(best.revenue) + " plus billiard " + formatMoney(best.billiard_revenue) + ")"
			: "no sales in this period";
	return (
		"Daily sales chart showing revenue for the last " +
		data.length +
		" days. Total " +
		formatMoney(totalShop + totalBilliard) +
		" (shop " +
		formatMoney(totalShop) +
		" plus billiard " +
		formatMoney(totalBilliard) +
		"); " +
		detail +
		"."
	);
}

function navigateToDate(dateStr) {
	window.location.href = "/sales?date=" + encodeURIComponent(dateStr);
}

function wireBarInteractivity(rect, entry) {
	rect.addEventListener("mouseenter", function () {
		showTooltip(entry, rect);
	});
	rect.addEventListener("mouseleave", hideTooltip);
	rect.addEventListener("focus", function () {
		showTooltip(entry, rect);
	});
	rect.addEventListener("blur", hideTooltip);
	rect.addEventListener("keydown", function (event) {
		if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			navigateToDate(entry.date);
		}
	});
	rect.addEventListener("click", function () {
		navigateToDate(entry.date);
	});
	// Touch has no hover: the first tap shows the tooltip (like a
	// mouse hover would) instead of immediately navigating away; a
	// second tap on the same bar lets the click through to navigate.
	rect.addEventListener("touchstart", function (event) {
		if (chartActiveTouchBar === rect) return;
		event.preventDefault();
		chartActiveTouchBar = rect;
		showTooltip(entry, rect);
	});
}

document.addEventListener("touchstart", function (event) {
	if (chartActiveTouchBar && !event.target.closest(".chart-bar")) {
		chartActiveTouchBar = null;
		hideTooltip();
	}
});

function buildTooltipEl() {
	var tooltip = document.createElement("div");
	tooltip.id = "chart-tooltip";
	tooltip.className = "chart-tooltip hidden";
	tooltip.setAttribute("role", "tooltip");
	return tooltip;
}

function showTooltip(entry, rect) {
	var tooltip = document.getElementById("chart-tooltip");
	var container = document.getElementById("chart-container");
	if (!tooltip || !container) return;

	tooltip.textContent = "";
	[
		fullDateLabel(entry.date),
		"Shop: " + formatMoney(entry.revenue),
		"Billiard: " + formatMoney(entry.billiard_revenue),
		"Total: " + formatMoney(entry.revenue + entry.billiard_revenue),
		"Units: " + entry.units,
		"Sales: " + entry.sale_count,
	].forEach(function (text) {
		var line = document.createElement("div");
		line.textContent = text;
		tooltip.appendChild(line);
	});

	var containerRect = container.getBoundingClientRect();
	var barRect = rect.getBoundingClientRect();
	tooltip.style.left = barRect.left - containerRect.left + barRect.width / 2 + "px";
	tooltip.style.top = barRect.top - containerRect.top + "px";
	tooltip.classList.remove("hidden");
}

function hideTooltip() {
	var tooltip = document.getElementById("chart-tooltip");
	if (tooltip) tooltip.classList.add("hidden");
}

// buildFallbackTable is a visually-hidden (but screen-reader visible)
// table mirroring the chart's data, independent of which metric is
// currently toggled on.
function buildFallbackTable(data) {
	var wrap = document.createElement("div");
	wrap.className = "sr-only";

	var table = document.createElement("table");
	var caption = document.createElement("caption");
	caption.textContent = "Daily sales data";
	table.appendChild(caption);

	var thead = document.createElement("thead");
	var headRow = document.createElement("tr");
	["Date", "Shop revenue", "Billiard revenue", "Total revenue", "Units", "Sales"].forEach(function (text) {
		var th = document.createElement("th");
		th.textContent = text;
		headRow.appendChild(th);
	});
	thead.appendChild(headRow);
	table.appendChild(thead);

	var tbody = document.createElement("tbody");
	data.forEach(function (entry) {
		var tr = document.createElement("tr");
		addCell(tr, fullDateLabel(entry.date));
		addCell(tr, formatMoney(entry.revenue));
		addCell(tr, formatMoney(entry.billiard_revenue));
		addCell(tr, formatMoney(entry.revenue + entry.billiard_revenue));
		addCell(tr, String(entry.units));
		addCell(tr, String(entry.sale_count));
		tbody.appendChild(tr);
	});
	table.appendChild(tbody);

	wrap.appendChild(table);
	return wrap;
}

function addCell(tr, text) {
	var td = document.createElement("td");
	td.textContent = text;
	tr.appendChild(td);
}
