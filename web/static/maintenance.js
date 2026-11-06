// Controls in the top bar that are on every page: the language switch,
// restoring a backup, and (in a browser) quitting. (The update button is update.js.)

(function () {
	"use strict";

	// el builds an element with a class and text; no innerHTML anywhere, so
	// file names and server messages can never be read as markup.
	function el(tag, className, text) {
		var node = document.createElement(tag);
		if (className) node.className = className;
		if (text != null) node.textContent = text;
		return node;
	}

	function button(label, className, onClick) {
		var b = el("button", "btn " + className, label);
		b.type = "button";
		b.onclick = onClick;
		return b;
	}

	// openPanel shows content in its own <dialog>, separate from the shared
	// form modal so a confirmation can open on top of it. build(body, close)
	// fills it and returns nothing; the dialog removes itself when closed.
	function openPanel(title, build) {
		var dlg = el("dialog", "modal no-print");
		dlg.appendChild(el("h2", null, title));
		var body = el("div", "panel-body");
		dlg.appendChild(body);
		dlg.addEventListener("close", function () {
			dlg.remove();
		});
		document.body.appendChild(dlg);
		build(body, function () {
			dlg.close();
		});
		dlg.showModal();
		return dlg;
	}

	function getJSON(path) {
		return apiFetch(path);
	}

	// ---- language -------------------------------------------------------

	var langSelect = document.getElementById("lang-select");
	if (langSelect) {
		langSelect.addEventListener("change", function () {
			document.cookie = "lang=" + encodeURIComponent(langSelect.value) + "; path=/; max-age=31536000; SameSite=Lax";
			window.location.reload();
		});
	}

	// ---- waiting for the app to restart ---------------------------------

	// reloadWhenRestarted polls until the server answers with a different
	// instance id (a new process), then reloads the page. It gives up after
	// about two minutes and says so through onGiveUp.
	function reloadWhenRestarted(oldInstance, onGiveUp) {
		var deadline = Date.now() + 120000;
		function poll() {
			if (Date.now() > deadline) {
				onGiveUp();
				return;
			}
			fetch("/api/version", { headers: { Accept: "application/json" }, cache: "no-store" })
				.then(function (res) {
					return res.ok ? res.json() : null;
				})
				.then(function (v) {
					if (v && v.instance !== oldInstance) {
						window.location.reload();
						return;
					}
					setTimeout(poll, 1000);
				})
				.catch(function () {
					setTimeout(poll, 1000);
				});
		}
		setTimeout(poll, 1500);
	}

	// ---- restore --------------------------------------------------------

	document.getElementById("restore-btn").addEventListener("click", function () {
		openPanel(t("restore.title"), buildRestorePanel);
	});

	function formatBytes(n) {
		if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + " MB";
		return Math.max(1, Math.round(n / 1024)) + " KB";
	}

	function buildRestorePanel(body, close) {
		body.appendChild(el("p", "panel-warning", t("restore.warning")));

		body.appendChild(el("h3", null, t("restore.from_list")));
		var list = el("div", "backup-list");
		body.appendChild(list);

		body.appendChild(el("h3", null, t("restore.from_file")));
		var fileInput = el("input");
		fileInput.type = "file";
		fileInput.accept = ".db,application/octet-stream";
		fileInput.setAttribute("aria-label", t("restore.from_file"));
		body.appendChild(fileInput);

		var error = el("p", "error", "");
		error.setAttribute("role", "alert");
		body.appendChild(error);
		var status = el("p", "panel-status", "");
		status.setAttribute("role", "status");
		body.appendChild(status);

		var actions = el("div", "modal-actions");
		var restoreBtn = button(t("restore.button"), "btn-danger", function () {
			chosenAndConfirm();
		});
		restoreBtn.disabled = true;
		var closeBtn = button(t("common.close"), "btn-ghost", close);
		actions.appendChild(restoreBtn);
		actions.appendChild(closeBtn);
		body.appendChild(actions);

		var selectedName = null;

		// The id of the running process, to tell the restarted one from it.
		var instance = null;
		getJSON("/api/version")
			.then(function (v) {
				instance = v.instance;
			})
			.catch(function () {});

		function refreshButton() {
			restoreBtn.disabled = !(selectedName || fileInput.files.length > 0);
		}

		fileInput.addEventListener("change", function () {
			if (fileInput.files.length > 0) {
				selectedName = null;
				list.querySelectorAll("input[type=radio]").forEach(function (r) {
					r.checked = false;
				});
			}
			refreshButton();
		});

		getJSON("/api/backups")
			.then(function (res) {
				var backups = res.backups || [];
				if (backups.length === 0) {
					list.appendChild(el("p", "empty", t("restore.none")));
					return;
				}
				backups.forEach(function (b) {
					var row = el("label", "backup-row");
					var radio = el("input");
					radio.type = "radio";
					radio.name = "backup";
					radio.value = b.name;
					radio.onchange = function () {
						selectedName = b.name;
						fileInput.value = "";
						refreshButton();
					};
					row.appendChild(radio);
					row.appendChild(el("span", "backup-name", b.name));
					row.appendChild(el("span", "backup-meta", new Date(b.modified * 1000).toLocaleString(localeTag()) + " · " + formatBytes(b.size)));
					list.appendChild(row);
				});
			})
			.catch(function (err) {
				error.textContent = err.message;
			});

		function chosenAndConfirm() {
			error.textContent = "";
			var file = fileInput.files.length > 0 ? fileInput.files[0] : null;
			var what = file ? file.name : selectedName;
			confirmDialog(t("restore.confirm", { name: what }), { title: t("restore.title"), confirmLabel: t("restore.button") }).then(function (ok) {
				if (ok) doRestore(file);
			});
		}

		function doRestore(file) {
			restoreBtn.disabled = closeBtn.disabled = true;
			status.textContent = t("restore.working");
			var request = file
				? fetch("/api/restore/upload", {
						method: "POST",
						headers: { "Content-Type": "application/octet-stream", Accept: "application/json" },
						body: file,
					}).then(function (res) {
						return res
							.json()
							.catch(function () {
								return {};
							})
							.then(function (b) {
								if (!res.ok) throw new Error(b.error || t("error.request_failed", { status: res.status }));
								return b;
							});
					})
				: apiFetch("/api/restore", { method: "POST", body: JSON.stringify({ name: selectedName }) });

			request
				.then(function () {
					status.textContent = t("restore.restarting");
					reloadWhenRestarted(instance, function () {
						status.textContent = t("restore.no_return");
						closeBtn.disabled = false;
					});
				})
				.catch(function (err) {
					status.textContent = "";
					error.textContent = err.message;
					closeBtn.disabled = false;
					refreshButton();
				});
		}
	}

	// ---- quit (browser mode only) ---------------------------------------

	var quitBtn = document.getElementById("quit-btn");
	window.configReady.then(function (cfg) {
		if (!cfg.can_quit) return;
		quitBtn.classList.remove("hidden");
		quitBtn.addEventListener("click", function () {
			confirmDialog(t("quit.confirm"), { title: t("quit.title"), confirmLabel: t("nav.quit") }).then(function (ok) {
				if (!ok) return;
				apiFetch("/api/quit", { method: "POST", body: "{}" })
					.then(function () {
						document.body.textContent = "";
						var msg = el("main", "page");
						msg.appendChild(el("h1", null, t("quit.stopped")));
						msg.appendChild(el("p", null, t("quit.close_tab")));
						document.body.appendChild(msg);
					})
					.catch(showError);
			});
		});
	});
})();
