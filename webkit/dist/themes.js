"use strict";
(() => {
  // src/themes-page.ts
  var SWATCH_ROLES = ["bg", "paper", "text-1", "primary", "series-2", "series-3"];
  function card(f, theme, v) {
    const el = Webkit.el;
    const swatches = el(
      "span",
      { class: "wk-theme-swatches" },
      SWATCH_ROLES.map((r) => el("i", { title: r, style: `background:${v.roles[r]}` }))
    );
    const code = el("code", {}, [
      el("span", { class: "k" }, "func"),
      " f(",
      el("span", { class: "n" }, "42"),
      ", ",
      el("span", { class: "s" }, '"ok"'),
      ")"
    ]);
    const sample = el("span", { class: "wk-theme-sample" }, [
      el("wk-badge", { variant: "a" }, "badge"),
      el("wk-badge", { variant: "ok" }, "ok"),
      el("wk-badge", { variant: "accent" }, "accent"),
      code,
      el("wk-callout", { variant: "info" }, `${v.variant}: a callout in this palette.`)
    ]);
    const source = el("a", { href: f.source, target: "_blank", rel: "noopener" }, "source");
    source.addEventListener("click", (e) => e.stopPropagation());
    const c = el("div", {
      class: "wk-theme-card",
      role: "button",
      tabindex: "0",
      "data-palette": f.name,
      "data-theme": theme,
      "data-family": f.name,
      "aria-pressed": "false",
      title: v.notes ?? ""
    }, [
      el("span", { class: "wk-theme-card-head" }, [
        el("span", {}, [
          el("span", { class: "wk-theme-card-name" }, f.label),
          " ",
          el("span", { class: "wk-theme-card-variant" }, v.variant)
        ]),
        el("span", { class: "wk-theme-card-current", hidden: "" }, "current")
      ]),
      swatches,
      sample,
      el("span", { class: "wk-theme-card-license" }, [`${f.license} \xB7 `, source])
    ]);
    const pick = () => {
      Webkit.setPalette(theme, f.name);
    };
    c.addEventListener("click", pick);
    c.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        pick();
      }
    });
    return c;
  }
  function reflect() {
    const s = Webkit.themeState();
    for (const theme of ["light", "dark"]) {
      const chosen = Webkit.paletteFor(theme);
      document.querySelectorAll(`#wk-theme-${theme} .wk-theme-card`).forEach((c) => {
        const on = c.dataset.family === chosen;
        c.setAttribute("aria-pressed", String(on));
        const tag = c.querySelector(".wk-theme-card-current");
        if (tag) {
          if (on) tag.removeAttribute("hidden");
          else tag.setAttribute("hidden", "");
        }
      });
    }
    const system = document.getElementById("wk-theme-system");
    if (system) system.checked = s.mode === "system";
    const status = document.getElementById("wk-theme-status");
    if (status) {
      const modeText = s.mode === "system" ? `system (${s.theme} now)` : s.mode;
      status.textContent = `Mode: ${modeText} \xB7 light: ${Webkit.paletteFor("light")} \xB7 dark: ${Webkit.paletteFor("dark")}`;
    }
  }
  function render(col) {
    for (const theme of ["light", "dark"]) {
      const host = document.getElementById(`wk-theme-${theme}`);
      if (!host) continue;
      host.replaceChildren();
      const offered = col.families.filter((f) => f[theme]);
      if (!offered.length) {
        host.appendChild(Webkit.el("p", { class: "wk-theme-empty" }, `No family ships a ${theme} variant.`));
        continue;
      }
      for (const f of offered) host.appendChild(card(f, theme, f[theme]));
    }
    reflect();
  }
  function wireControls() {
    const system = document.getElementById("wk-theme-system");
    system?.addEventListener("change", () => {
      Webkit.setThemeMode(system.checked ? "system" : Webkit.themeState().theme);
    });
    const reset = document.getElementById("wk-theme-reset");
    const doReset = () => {
      Webkit.resetTheme();
    };
    reset?.addEventListener("click", doReset);
    reset?.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        doReset();
      }
    });
    document.addEventListener("wk-themechange", reflect);
  }
  async function main() {
    wireControls();
    const res = await fetch("/webkit/themes.json", { headers: { accept: "application/json" } });
    if (!res.ok) throw new Error(`themes.json failed (${res.status})`);
    render(await res.json());
  }
  main().catch((err) => {
    const msg = err instanceof Error ? err.message : String(err);
    for (const id of ["wk-theme-light", "wk-theme-dark"]) {
      const host = document.getElementById(id);
      if (host) host.replaceChildren(Webkit.el("p", { class: "wk-theme-empty" }, `Could not load the theme collection: ${msg}`));
    }
  });
})();
