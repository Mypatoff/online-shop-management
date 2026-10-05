// Billiard page: a quick entry form for table-time payments (no
// stock involved), plus a date-navigable daily history with void and
// receipt printing.

var billiardCurrentDate = formatDateInput(new Date());

var amountInput = document.getElementById("billiard-amount-input");
var tableInput = document.getElementById("billiard-table-input");
var minutesInput = document.getElementById("billiard-minutes-input");
var noteInput = document.getElementById("billiard-note-input");
var saveBtn = document.getElementById("billiard-save-btn");
var entryErrorEl = document.getElementById("billiard-entry-error");
var entryForm = document.getElementById("billiard-entry-form");
var tableSuggestions = document.getElementById("billiard-table-suggestions");

var dateInput = document.getElementById("billiard-date-input");
var prevBtn = document.getElementById("billiard-prev-day-btn");
var nextBtn = document.getElementById("billiard-next-day-btn");

dateInput.value = billiardCurrentDate;

window.configReady.then(function () {
	loadDay(billiardCurrentDate);
});

// The amount input autofocuses once config (and so formatMoney) is
// ready, matching the "focus the amount input" behavior after saving.
window.configReady.then(function () {
	amountInput.focus();
});

entryForm.addEventListener("submit", function (event) {
	event.preventDefault();
	submitEntry();
});

dateInput.addEventListener("change", function () {
	if (dateInput.value) loadDay(dateInput.value);
});

prevBtn.addEventListener("click", function () {
	loadDay(shiftDate(billiardCurrentDate, -1));
});

nextBtn.addEventListener("click", function () {
	loadDay(shiftDate(billiardCurrentDate, 1));
});

function submitEntry() {
	entryErrorEl.textContent = "";

	var amount;
	try {
		amount = parseMoney(amountInput.value);
	} catch (err) {
		entryErrorEl.textContent = err.message;
		amountInput.focus();
		return;
	}

	var table = tableInput.value.trim();
	if (table.length > 30) {
		entryErrorEl.textContent = "Table must be at most 30 characters.";
		tableInput.focus();
		return;
	}

	var minutes = null;
	var minutesRaw = minutesInput.value.trim();
	if (minutesRaw !== "") {
		try {
			minutes = parseWholeNumber(minutesRaw, { message: "Minutes must be a whole number." });
		} catch (err) {
			entryErrorEl.textContent = err.message;
			minutesInput.focus();
			return;
		}
		if (minutes < 0 || minutes > 1440) {
			entryErrorEl.textContent = "Minutes must be between 0 and 1440.";
			minutesInput.focus();
			return;
		}
	}

	var note = noteInput.value.trim();
	if (note.length > 200) {
		entryErrorEl.textContent = "Note must be at most 200 characters.";
		noteInput.focus();
		return;
	}

	saveBtn.disabled = true;
	apiFetch("/api/billiard", {
		method: "POST",
		body: JSON.stringify({ amount: amount, table: table, minutes: minutes, note: note }),
	})
		.then(function (entry) {
			showToast("Saved " + formatMoney(entry.amount));
			amountInput.value = "";
			noteInput.value = "";
			amountInput.focus();
			loadDay(billiardCurrentDate);
		})
		.catch(function (err) {
			entryErrorEl.textContent = err.message;
		})
		.finally(function () {
			saveBtn.disabled = false;
		});
}

function loadDay(dateStr) {
	billiardCurrentDate = dateStr;
	dateInput.value = dateStr;
	updateNextDisabled();

	apiFetch("/api/billiard?date=" + encodeURIComponent(dateStr)).then(function (data) {
		renderTotals(data.totals);
		renderTableSuggestions(data.tables);
		renderEntries(data.entries);
	});
}

function updateNextDisabled() {
	nextBtn.disabled = billiardCurrentDate >= formatDateInput(new Date());
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

function renderTotals(totals) {
	document.getElementById("billiard-day-revenue").textContent = formatMoney(totals.revenue);
	document.getElementById("billiard-day-count").textContent = totals.count;
}

function renderTableSuggestions(tables) {
	tableSuggestions.textContent = "";
	(tables || []).forEach(function (name) {
		var option = document.createElement("option");
		option.value = name;
		tableSuggestions.appendChild(option);
	});
}

function renderEntries(entries) {
	var tbody = document.getElementById("billiard-tbody");
	var empty = document.getElementById("billiard-empty");
	var wrap = document.getElementById("billiard-wrap");
	tbody.textContent = "";

	if (!entries || entries.length === 0) {
		empty.classList.remove("hidden");
		wrap.classList.add("hidden");
		return;
	}
	empty.classList.add("hidden");
	wrap.classList.remove("hidden");

	entries.forEach(function (entry) {
		tbody.appendChild(buildEntryRow(entry));
	});
}

function buildEntryRow(entry) {
	var tr = document.createElement("tr");
	if (entry.voided) tr.className = "voided-row";

	addCell(tr, formatTimeOfDay(entry.created_at));
	addCell(tr, entry.table || "—");
	addCell(tr, entry.minutes != null ? String(entry.minutes) : "—");
	addCell(tr, entry.note || "—");
	addCell(tr, formatMoney(entry.amount));
	addCell(tr, entry.voided ? "Voided" : "Completed");

	var actionsTd = document.createElement("td");
	actionsTd.className = "row-actions";
	if (!entry.voided) {
		var voidBtn = document.createElement("button");
		voidBtn.type = "button";
		voidBtn.className = "btn btn-secondary btn-small";
		voidBtn.textContent = "Void";
		voidBtn.onclick = function () {
			if (!confirm("Void this billiard entry?")) return;
			apiFetch("/api/billiard/" + entry.id + "/void", { method: "POST", body: "{}" })
				.then(function () {
					loadDay(billiardCurrentDate);
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
			printBilliardReceipt(entry);
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

// printBilliardReceipt fills the print-only #billiard-receipt block
// for one entry, reusing the Sales page's till-receipt printing
// approach (narrow @page size, removed again after printing).
function printBilliardReceipt(entry) {
	var container = document.getElementById("billiard-receipt");
	container.textContent = "";
	container.className = "print-only receipt";

	var shopName = (window.appConfig && window.appConfig.shop_name) || "ShopKeeper";

	appendParagraph(container, shopName);
	appendParagraph(container, new Date(entry.created_at * 1000).toLocaleString());
	appendParagraph(container, "Billiard");
	if (entry.table) appendParagraph(container, "Table: " + entry.table);
	if (entry.minutes != null) appendParagraph(container, "Minutes: " + entry.minutes);
	appendParagraph(container, "Amount: " + formatMoney(entry.amount));
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

function appendParagraph(container, text) {
	var p = document.createElement("p");
	p.textContent = text;
	container.appendChild(p);
}
