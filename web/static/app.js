// Shared helpers used by every page: talking to the JSON API,
// formatting/parsing money, and a generic modal dialog.

// Loaded once and cached; pages await this before they format or
// parse any money value.
window.configReady = apiFetch("/api/config").then(function (cfg) {
	window.appConfig = cfg;
	return cfg;
});

// apiFetch sends a JSON request and resolves with the parsed JSON
// body. On a non-2xx response it rejects with an Error carrying the
// server's error message, so callers can show it to the user.
function apiFetch(path, options) {
	options = options || {};
	var headers = Object.assign({ Accept: "application/json" }, options.headers || {});
	if (options.body !== undefined) {
		headers["Content-Type"] = "application/json";
	}
	return fetch(path, Object.assign({}, options, { headers: headers })).then(function (res) {
		return res
			.json()
			.catch(function () {
				return {};
			})
			.then(function (body) {
				if (!res.ok) {
					throw new Error(body.error || "Request failed (" + res.status + ").");
				}
				return body;
			});
	});
}

// NBSP (U+00A0) separates thousands groups, and separates the trailing
// currency label from the number — using a non-breaking space (rather
// than a regular one) keeps "150 000 so'm" from ever wrapping onto two
// lines.
var NBSP = " ";

// groupThousands inserts NBSP every 3 digits from the right, e.g.
// "150000" -> "150 000". digits must be a plain non-negative
// integer string (no sign, no separators).
function groupThousands(digits) {
	var out = "";
	for (var i = 0; i < digits.length; i++) {
		if (i > 0 && (digits.length - i) % 3 === 0) out += NBSP;
		out += digits[i];
	}
	return out;
}

// formatMoney turns an integer amount in minor units (whole so'm when
// DECIMALS=0) into a display string like "150 000 so'm",
// using only integer math: no parseFloat, no division that could lose
// precision. Negative amounts get a leading minus sign.
function formatMoney(amount) {
	var cfg = window.appConfig || { currency: "", decimals: 0 };
	var decimals = cfg.decimals;
	var negative = amount < 0;
	var abs = Math.abs(amount);

	var scale = 1;
	for (var i = 0; i < decimals; i++) scale *= 10;

	var whole = Math.floor(abs / scale);
	var frac = abs - whole * scale;
	var fracStr = String(frac);
	while (fracStr.length < decimals) fracStr = "0" + fracStr;

	var display = groupThousands(String(whole));
	if (decimals > 0) display += "," + fracStr;

	var withSign = (negative ? "-" : "") + display;
	return cfg.currency ? withSign + NBSP + cfg.currency : withSign;
}

// parseMoney turns user input into an integer amount in minor units.
// It never calls parseFloat. With DECIMALS=0 (the default) it accepts
// only digits and plain/non-breaking spaces as thousands separators;
// any "." or "," is rejected outright with a message steering the user
// toward whole so'm. With DECIMALS>0 it keeps the previous behavior:
// splitting on "," or "." and parsing each half with parseInt. Throws
// an Error with a user-facing message on invalid input.
function parseMoney(input) {
	var cfg = window.appConfig || { decimals: 0 };
	var decimals = cfg.decimals;
	var stripped = String(input).trim().replace(/\s/g, "");

	if (stripped === "") throw new Error("Enter an amount.");

	if (decimals === 0) {
		if (stripped.indexOf(".") !== -1 || stripped.indexOf(",") !== -1) {
			throw new Error("Enter whole so'm without decimals, e.g. 150000");
		}
		if (!/^\d+$/.test(stripped)) {
			throw new Error("Enter whole so'm without decimals, e.g. 150000");
		}
		return parseInt(stripped, 10);
	}

	var s = stripped.replace(",", ".");
	var parts = s.split(".");
	if (parts.length > 2) throw new Error("Amount has too many decimal points.");

	var wholePart = parts[0];
	var fracPart = parts.length === 2 ? parts[1] : "";

	if (!/^\d+$/.test(wholePart)) throw new Error("Amount must be a plain number.");
	if (fracPart !== "" && !/^\d+$/.test(fracPart)) throw new Error("Amount must be a plain number.");
	if (fracPart.length > decimals) {
		throw new Error("Amount allows at most " + decimals + " decimal place" + (decimals === 1 ? "" : "s") + ".");
	}
	while (fracPart.length < decimals) fracPart += "0";

	var scale = 1;
	for (var i = 0; i < decimals; i++) scale *= 10;

	var wholeNum = parseInt(wholePart, 10);
	var fracNum = fracPart === "" ? 0 : parseInt(fracPart, 10);
	return wholeNum * scale + fracNum;
}

// parseWholeNumber validates that input is a plain (optionally
// negative) integer and returns it, or throws an Error. Used for
// stock/threshold/delta fields, which never accept decimals.
function parseWholeNumber(input, opts) {
	opts = opts || {};
	var s = String(input).trim();
	var pattern = opts.allowNegative ? /^-?\d+$/ : /^\d+$/;
	if (!pattern.test(s)) {
		throw new Error(opts.message || "Enter a whole number.");
	}
	return parseInt(s, 10);
}

// fieldError builds an Error tagged with which form field it applies
// to, so openModal can show it next to that field instead of as a
// generic message.
function fieldError(field, message) {
	var err = new Error(message);
	err.field = field;
	return err;
}

// openModal renders a small form into the shared <dialog id="modal">
// and wires up submit/cancel. opts:
//   title: string
//   submitLabel: string (default "Save")
//   fields: [{ name, label, type, value, autofocus }]
//   onSubmit: function(values) -> Promise, rejected with a fieldError
//             or plain Error on failure, resolved on success.
function openModal(opts) {
	var modal = document.getElementById("modal");
	var form = document.getElementById("modal-form");
	var title = document.getElementById("modal-title");
	var body = document.getElementById("modal-body");
	var errorEl = document.getElementById("modal-error");
	var submitBtn = document.getElementById("modal-submit");
	var cancelBtn = document.getElementById("modal-cancel");

	title.textContent = opts.title;
	errorEl.textContent = "";
	submitBtn.textContent = opts.submitLabel || "Save";
	body.textContent = "";

	var inputs = {};
	opts.fields.forEach(function (f) {
		var wrap = document.createElement("label");
		wrap.className = "field";

		var span = document.createElement("span");
		span.textContent = f.label;
		wrap.appendChild(span);

		var input;
		if (f.type === "select") {
			input = document.createElement("select");
			(f.options || []).forEach(function (opt) {
				var option = document.createElement("option");
				option.value = opt.value;
				option.textContent = opt.label;
				input.appendChild(option);
			});
			input.value = f.value != null ? f.value : "";
		} else {
			input = document.createElement("input");
			input.type = f.type || "text";
			input.value = f.value != null ? f.value : "";
		}
		input.name = f.name;
		wrap.appendChild(input);

		var fieldErrorEl = document.createElement("div");
		fieldErrorEl.className = "field-error";
		fieldErrorEl.setAttribute("data-field-error-for", f.name);
		wrap.appendChild(fieldErrorEl);

		body.appendChild(wrap);
		inputs[f.name] = input;
	});

	function clearErrors() {
		errorEl.textContent = "";
		body.querySelectorAll(".field-error").forEach(function (el) {
			el.textContent = "";
		});
	}

	function showError(err) {
		var message = err && err.message ? err.message : String(err);
		var target = err && err.field && body.querySelector('[data-field-error-for="' + err.field + '"]');
		if (target) {
			target.textContent = message;
		} else {
			errorEl.textContent = message;
		}
	}

	form.onsubmit = function (event) {
		event.preventDefault();
		clearErrors();
		var values = {};
		Object.keys(inputs).forEach(function (name) {
			values[name] = inputs[name].value;
		});
		Promise.resolve()
			.then(function () {
				return opts.onSubmit(values);
			})
			.then(function () {
				modal.close();
			})
			.catch(showError);
	};

	cancelBtn.onclick = function () {
		modal.close();
	};

	modal.showModal();
	var first = opts.fields[0];
	if (first) inputs[first.name].focus();
}

// formatLocalTime turns a unix-seconds timestamp into the user's local
// date/time string, for display in sales lists.
function formatLocalTime(unixSeconds) {
	return new Date(unixSeconds * 1000).toLocaleString();
}

// showToast briefly shows a message in the shared #toast element.
var toastTimer = null;
function showToast(message) {
	var el = document.getElementById("toast");
	if (!el) return;
	el.textContent = message;
	el.classList.remove("hidden");
	clearTimeout(toastTimer);
	toastTimer = setTimeout(function () {
		el.classList.add("hidden");
	}, 3000);
}

// openAdjustStockModal is shared by the dashboard's low-stock list and
// the products table: both let the user change a product's stock by
// a positive or negative amount.
var adjustReasonOptions = [
	{ value: "restock", label: "Restock" },
	{ value: "recount", label: "Recount" },
	{ value: "damaged", label: "Damaged" },
	{ value: "other", label: "Other" },
];

function openAdjustStockModal(product, onSuccess) {
	openModal({
		title: "Adjust stock — " + product.name,
		submitLabel: "Apply",
		fields: [
			{ name: "delta", label: "Change stock by (e.g. 10 or -2)", autofocus: true },
			{ name: "reason", label: "Reason", type: "select", options: adjustReasonOptions, value: "restock" },
			{ name: "note", label: "Note (optional)" },
		],
		onSubmit: function (values) {
			var delta;
			try {
				delta = parseWholeNumber(values.delta, { allowNegative: true, message: "Enter a whole number like 10 or -2." });
			} catch (err) {
				throw fieldError("delta", err.message);
			}
			if (delta === 0) throw fieldError("delta", "Enter a non-zero number.");

			var note = values.note.trim();
			if (note.length > 200) throw fieldError("note", "Note must be at most 200 characters.");

			return apiFetch("/api/products/" + product.id + "/adjust", {
				method: "POST",
				body: JSON.stringify({ delta: delta, reason: values.reason, note: note }),
			}).then(function (res) {
				if (onSuccess) onSuccess(res.stock);
			});
		},
	});
}
