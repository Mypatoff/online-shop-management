// The navbar's version label and "Update" button: asks GitHub (through the local server)
// whether a newer release exists, and installs it. The app keeps working offline; nothing
// here runs until the button is clicked.

(function () {
	var button = document.getElementById("update-btn");
	var dialog = document.getElementById("update-dialog");
	if (!button || !dialog) return;

	var body = document.getElementById("update-body");
	var goBtn = document.getElementById("update-go");
	var closeBtn = document.getElementById("update-close");
	var versionEl = document.getElementById("app-version");
	var running = false; // an update is being installed: the dialog cannot be closed

	function label(v) {
		return /^\d/.test(v) ? "v" + v : v;
	}

	apiFetch("/api/version")
		.then(function (res) {
			versionEl.textContent = label(res.version);
		})
		.catch(function () {});

	function show() {
		body.textContent = "";
		for (var i = 0; i < arguments.length; i++) {
			if (arguments[i]) body.appendChild(arguments[i]);
		}
	}

	function para(text, className) {
		var p = document.createElement("p");
		p.textContent = text;
		if (className) p.className = className;
		return p;
	}

	function progress(percent) {
		var bar = document.createElement("div");
		bar.className = "update-progress";
		bar.setAttribute("role", "progressbar");
		bar.setAttribute("aria-valuemin", "0");
		bar.setAttribute("aria-valuemax", "100");
		bar.setAttribute("aria-valuenow", String(percent));
		var fill = document.createElement("div");
		fill.style.width = percent + "%";
		bar.appendChild(fill);
		return bar;
	}

	function setButtons(opts) {
		goBtn.hidden = !opts.go;
		goBtn.disabled = false;
		goBtn.textContent = opts.goLabel || t("update.go");
		closeBtn.hidden = !!opts.hideClose;
		closeBtn.textContent = opts.closeLabel || t("common.close");
	}

	function check() {
		setButtons({});
		show(para(t("update.checking")));
		apiFetch("/api/update/check")
			.then(showResult)
			.catch(function (err) {
				show(para(err.message, "error"));
				setButtons({ go: true, goLabel: t("update.try_again") });
				goBtn.onclick = check;
			});
	}

	function showResult(info) {
		var current = label(info.current);
		if (!info.available) {
			show(
				para(info.reason || t("update.latest"), null),
				para(info.latest ? t("update.installed_latest", { current: current, latest: label(info.latest) }) : t("update.installed", { current: current }))
			);
			setButtons({});
			return;
		}
		var notes = null;
		if (info.notes) {
			notes = document.createElement("div");
			notes.className = "update-notes";
			notes.textContent = info.notes;
		}
		if (!info.can_apply) {
			show(
				para(t("update.available_line", { latest: label(info.latest), current: current })),
				para(info.reason, "error"),
				info.url ? para(t("update.release_page", { url: info.url })) : null
			);
			setButtons({});
			return;
		}
		show(
			para(t("update.available_line", { latest: label(info.latest), current: current })),
			notes,
			para(t("update.kept_note"))
		);
		setButtons({ go: true });
		goBtn.onclick = install;
	}

	function install() {
		running = true;
		goBtn.disabled = true;
		closeBtn.hidden = true;
		show(para(t("update.starting")), progress(0));
		apiFetch("/api/update/apply", { method: "POST", body: "{}" })
			.then(function () {
				poll(Date.now());
			})
			.catch(function (err) {
				fail(err.message);
			});
	}

	function fail(message) {
		running = false;
		show(para(t("update.failed_title"), null), para(message, "error"));
		setButtons({ go: true, goLabel: t("update.try_again") });
		goBtn.onclick = check;
	}

	// poll follows the download. Once the server reports "restarting" it goes away for a few
	// seconds, so failed requests are expected there; the page reloads when the new version
	// answers. (On Windows the window itself is replaced, and this code just stops.)
	function poll(startedAt) {
		apiFetch("/api/update/status")
			.then(function (st) {
				if (st.state === "error") {
					fail(st.error || t("error.unknown"));
				} else if (st.state === "restarting") {
					show(para(t("update.installing")), progress(100));
					waitForRestart(Date.now());
				} else {
					show(para(t("update.downloading", { pct: st.percent })), progress(st.percent));
					setTimeout(function () {
						poll(startedAt);
					}, 500);
				}
			})
			.catch(function () {
				// The server may already be shutting down for the restart.
				show(para(t("update.installing")), progress(100));
				waitForRestart(Date.now());
			});
	}

	function waitForRestart(startedAt) {
		setTimeout(function () {
			fetch("/api/version", { cache: "no-store" })
				.then(function (res) {
					return res.ok ? res.json() : Promise.reject(new Error("not ready"));
				})
				.then(function (res) {
					var before = versionEl.textContent;
					if (label(res.version) !== before) {
						location.reload();
					} else if (Date.now() - startedAt < 15000) {
						waitForRestart(startedAt); // still the old server shutting down
					} else {
						running = false;
						show(para(t("update.same_version", { v: before })), para(t("update.check_log"), "error"));
						setButtons({});
					}
				})
				.catch(function () {
					if (Date.now() - startedAt > 90000) {
						running = false;
						show(para(t("update.no_return"), "error"));
						setButtons({});
					} else {
						waitForRestart(startedAt);
					}
				});
		}, 1000);
	}

	button.addEventListener("click", function () {
		dialog.showModal();
		check();
	});
	closeBtn.addEventListener("click", function () {
		dialog.close();
	});
	// Esc must not dismiss the dialog halfway through an update.
	dialog.addEventListener("cancel", function (event) {
		if (running) event.preventDefault();
	});
})();
