// Shared helpers used by every page: translations, talking to the JSON API,
// formatting/parsing money, and a generic modal dialog.

// t looks up a UI string in the table the server embedded in the page
// (window.I18N, for the current language) and fills {name} placeholders from
// params. A key with no entry shows as the key itself, so a gap is visible.
function t(key, params) {
	var s = window.I18N && window.I18N[key] != null ? window.I18N[key] : key;
	if (params) {
		s = s.replace(/\{(\w+)\}/g, function (m, name) {
			return params[name] != null ? params[name] : m;
		});
	}
	return s;
}

// tOr is t for keys that may not exist (values that come from the server,
// such as a stock movement's reason): it returns fallback instead of the key.
function tOr(key, fallback) {
	return window.I18N && window.I18N[key] != null ? window.I18N[key] : fallback;
}

// localeTag is the BCP 47 tag dates are formatted with (en-GB, uz-UZ, ru-RU).
function localeTag() {
	return window.LOCALE_TAG || "en-GB";
}

// numberSeparators gives the thousands and decimal separators for the
// language: Uzbek and Russian group with a non-breaking space and use a
// decimal comma; English groups with commas and uses a decimal point.
function numberSeparators() {
	if (window.LANG === "en") return { group: ",", decimal: "." };
	return { group: NBSP, decimal: "," };
}

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
					throw new Error(body.error || t("error.request_failed", { status: res.status }));
				}
				return body;
			});
	});
}

// NBSP (U+00A0) separates the trailing currency label from the number, and
// (in Uzbek and Russian) thousands groups — using a non-breaking space
// (rather than a regular one) keeps "150 000 so'm" from ever wrapping onto
// two lines.
var NBSP = "\u00a0";

// groupThousands inserts the language's group separator every 3 digits
// from the right, e.g. "150000" -> "150 000" (or "150,000" in English).
// digits must be a plain non-negative integer string (no sign, no
// separators).
function groupThousands(digits) {
	var sep = numberSeparators().group;
	var out = "";
	for (var i = 0; i < digits.length; i++) {
		if (i > 0 && (digits.length - i) % 3 === 0) out += sep;
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
	if (decimals > 0) display += numberSeparators().decimal + fracStr;

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

	if (stripped === "") throw new Error(t("money.enter_amount"));

	if (decimals === 0) {
		if (stripped.indexOf(".") !== -1 || stripped.indexOf(",") !== -1) {
			throw new Error(t("money.whole_only"));
		}
		if (!/^\d+$/.test(stripped)) {
			throw new Error(t("money.whole_only"));
		}
		return parseInt(stripped, 10);
	}

	var s = stripped.replace(",", ".");
	var parts = s.split(".");
	if (parts.length > 2) throw new Error(t("money.too_many_points"));

	var wholePart = parts[0];
	var fracPart = parts.length === 2 ? parts[1] : "";

	if (!/^\d+$/.test(wholePart)) throw new Error(t("money.plain_number"));
	if (fracPart !== "" && !/^\d+$/.test(fracPart)) throw new Error(t("money.plain_number"));
	if (fracPart.length > decimals) {
		throw new Error(t("money.max_decimals", { n: decimals }));
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
		throw new Error(opts.message || t("number.whole"));
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
//   message: string, optional text shown above the fields
//   submitLabel: string (default "Save")
//   danger: true styles the submit button as a destructive action
//   fields: [{ name, label, type, value, autofocus }]
//   onSubmit: function(values) -> Promise, rejected with a fieldError
//             or plain Error on failure, resolved on success.
//   onClose: function(submitted) called once when the dialog closes, however
//            it closes (submitted, cancelled, Esc); submitted is true only
//            after onSubmit succeeded.
function openModal(opts) {
	var modal = document.getElementById("modal");
	var form = document.getElementById("modal-form");
	var title = document.getElementById("modal-title");
	var body = document.getElementById("modal-body");
	var errorEl = document.getElementById("modal-error");
	var submitBtn = document.getElementById("modal-submit");
	var cancelBtn = document.getElementById("modal-cancel");
	var messageEl = document.getElementById("modal-message");

	title.textContent = opts.title;
	errorEl.textContent = "";
	submitBtn.textContent = opts.submitLabel || t("common.save");
	submitBtn.className = "btn " + (opts.danger ? "btn-danger" : "btn-primary");
	cancelBtn.textContent = t("common.cancel");
	messageEl.textContent = opts.message || "";
	messageEl.classList.toggle("hidden", !opts.message);
	body.textContent = "";
	var submitted = false;

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

	function showFormError(err) {
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
				submitted = true;
				modal.close();
			})
			.catch(showFormError);
	};

	cancelBtn.onclick = function () {
		modal.close();
	};

	// "close" fires for every way out (submit, Cancel, Esc). Assigning the
	// property, not adding a listener, so a dialog opened later replaces this
	// one's handler instead of stacking on it.
	modal.onclose = function () {
		modal.onclose = null;
		if (opts.onClose) opts.onClose(submitted);
	};

	modal.showModal();
	var first = opts.fields[0];
	if (first) {
		inputs[first.name].focus();
	} else if (opts.danger) {
		cancelBtn.focus(); // a destructive default must not be one Enter away
	} else {
		submitBtn.focus();
	}
}

// confirmDialog asks the user to confirm an action in the app's own modal
// (not window.confirm, which blocks the page and may be suppressed by an
// embedded WebView). It resolves true only if they confirm.
//   opts: { title, confirmLabel, danger }
function confirmDialog(message, opts) {
	opts = opts || {};
	return new Promise(function (resolve) {
		openModal({
			title: opts.title || t("confirm.title"),
			message: message,
			submitLabel: opts.confirmLabel || t("common.confirm"),
			danger: opts.danger !== false,
			fields: [],
			onSubmit: function () {},
			onClose: function (submitted) {
				resolve(submitted);
			},
		});
	});
}

// formatLocalTime turns a unix-seconds timestamp into the user's local
// date/time string, for display in sales lists.
function formatLocalTime(unixSeconds) {
	return new Date(unixSeconds * 1000).toLocaleString(localeTag());
}

// formatDateDisplay turns a YYYY-MM-DD date (as the API uses) into the form
// people read in the current language: 05.10.2026 in Uzbek and Russian, the
// ISO form in English.
function formatDateDisplay(dateStr) {
	var parts = String(dateStr).split("-");
	if (window.LANG === "en" || parts.length !== 3) return dateStr;
	return parts[2] + "." + parts[1] + "." + parts[0];
}

// showToast briefly shows a message in the shared #toast element.
var toastTimer = null;
function showToast(message, opts) {
	var el = document.getElementById("toast");
	if (!el) return;
	opts = opts || {};
	el.textContent = message;
	el.classList.toggle("toast-error", !!opts.error);
	el.classList.remove("hidden");
	clearTimeout(toastTimer);
	toastTimer = setTimeout(function () {
		el.classList.add("hidden");
	}, opts.error ? 6000 : 3000);
}

// showError reports a failure (replacing window.alert): a red toast that
// stays a little longer than a success message.
function showError(err) {
	showToast(err && err.message ? err.message : String(err), { error: true });
}

// openAdjustStockModal is shared by the dashboard's low-stock list and
// the products table: both let the user change a product's stock by
// a positive or negative amount.
function adjustReasonOptions() {
	return ["restock", "recount", "damaged", "other"].map(function (value) {
		return { value: value, label: t("reason." + value) };
	});
}

function openAdjustStockModal(product, onSuccess) {
	openModal({
		title: t("adjust.title", { name: product.name }),
		submitLabel: t("adjust.apply"),
		fields: [
			{ name: "delta", label: t("adjust.delta"), autofocus: true },
			{ name: "reason", label: t("adjust.reason"), type: "select", options: adjustReasonOptions(), value: "restock" },
			{ name: "note", label: t("common.note_optional") },
		],
		onSubmit: function (values) {
			var delta;
			try {
				delta = parseWholeNumber(values.delta, { allowNegative: true, message: t("adjust.delta_invalid") });
			} catch (err) {
				throw fieldError("delta", err.message);
			}
			if (delta === 0) throw fieldError("delta", t("adjust.delta_zero"));

			var note = values.note.trim();
			if (note.length > 200) throw fieldError("note", t("error.note_length"));

			return apiFetch("/api/products/" + product.id + "/adjust", {
				method: "POST",
				body: JSON.stringify({ delta: delta, reason: values.reason, note: note }),
			}).then(function (res) {
				if (onSuccess) onSuccess(res.stock);
			});
		},
	});
}
