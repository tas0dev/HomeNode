const app = document.querySelector("#app");
const overlay = document.querySelector("#overlay");
let csrf = "";
let authenticated = false;
let connection = "接続中";
let status = {
    name: "HomeNode",
    state: "OFFLINE",
    last_heartbeat: null,
    stale: true,
    stale_after_seconds: 15,
    telemetry_interval_seconds: 1,
    operation: null,
};
let sample = null;
let samples = [];
let nodeSample = null;
let nodeHistory = [];
let stale = true;
let sunshine = {
    state: "UNKNOWN",
    lan_address: "",
    tailnet_address: "",
};
let logs = [];
let page = 1;
let moreLogs = false;
let logLevel = "";
let logComponent = "";
let expandedLog = "";
let requestBusy = false;
let feedback = "";
let sunshineExpanded = false;
let menuState = "";
let socket = null;
let reconnect = 1000;
let retryTimer;
let fetching = false;
let pending = false;
let generation = 0;
let lastSync = 0;
let logMarker = "";
let loadingLogs = false;
let pendingLogs = false;
const selectedSeries = new Map();
const selectedTime = new Map();
const categories = {
    cpu: {
        name: "CPU",
        series: [
            { key: "cpu.usage_percent", name: "使用率", unit: "%" },
            { key: "cpu.temperature_c", name: "温度", unit: "℃" },
            { key: "cpu.clock_mhz", name: "クロック", unit: "MHz" },
        ],
        info: ["cpu.model"],
    },
    gpu: {
        name: "GPU",
        series: [
            { key: "gpu.usage_percent", name: "使用率", unit: "%" },
            { key: "gpu.temperature_c", name: "温度", unit: "℃" },
            { key: "gpu.clock_mhz", name: "クロック", unit: "MHz" },
            { key: "gpu.vram_used_bytes", name: "VRAM使用量", unit: "GiB" },
            { key: "gpu.vram_total_bytes", name: "VRAM総容量", unit: "GiB" },
            { key: "gpu.power_w", name: "消費電力", unit: "W" },
        ],
        info: ["gpu.model"],
    },
    memory: {
        name: "Memory",
        series: [
            { key: "memory.used_bytes", name: "使用量", unit: "GiB" },
            { key: "memory.available_bytes", name: "空き容量", unit: "GiB" },
            { key: "memory.total_bytes", name: "総容量", unit: "GiB" },
            { key: "memory.usage_percent", name: "使用率", unit: "%" },
        ],
        info: [],
    },
    network: {
        name: "Network",
        series: [
            { key: "network.rx_bytes_per_sec", name: "受信速度", unit: "MiB/s" },
            { key: "network.tx_bytes_per_sec", name: "送信速度", unit: "MiB/s" },
            { key: "network.rx_total_bytes", name: "累計受信", unit: "GiB" },
            { key: "network.tx_total_bytes", name: "累計送信", unit: "GiB" },
            { key: "network.link_speed_bps", name: "リンク速度", unit: "Mbit/s" },
        ],
        info: ["network.interface", "network.ipv4"],
    },
    node: {
        name: "Raspberry Pi",
        series: [
            { key: "cpu.usage_percent", name: "CPU使用率", unit: "%" },
            { key: "cpu.temperature_c", name: "CPU温度", unit: "℃" },
            { key: "memory.used_bytes", name: "メモリ使用量", unit: "GiB" },
            { key: "memory.total_bytes", name: "メモリ総容量", unit: "GiB" },
        ],
        info: ["wifi", "uptime_seconds"],
    },
};
function element(tag, className = "", text = "") {
    const e = document.createElement(tag);
    e.className = className;
    if (text)
        e.textContent = text;
    return e;
}
function button(text, action, className = "") {
    const b = element("button", className, text);
    b.type = "button";
    b.onclick = action;
    return b;
}
function link(text, path, className = "") {
    const a = element("a", className, text);
    a.href = path;
    a.onclick = (e) => {
        if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey)
            return;
        e.preventDefault();
        navigate(path);
    };
    return a;
}
function navigate(path) {
    history.pushState({}, "", path);
    closeMenu();
    render();
    window.scrollTo(0, 0);
}
window.addEventListener("popstate", () => {
    closeMenu();
    render();
});
function at(s, key) {
    if (!s)
        return null;
    const [group, field] = key.split(".");
    if (!field)
        return s[group] ?? null;
    return (s[group]?.[field] ??
        null);
}
function converted(value, unit) {
    if (unit === "GiB")
        return value / 1073741824;
    if (unit === "MiB/s")
        return value / 1048576;
    if (unit === "Mbit/s")
        return value / 1000000;
    return value;
}
function format(value, unit = "") {
    if (value === null ||
        value === undefined ||
        (typeof value === "number" && !Number.isFinite(value)))
        return "—";
    if (typeof value === "string")
        return value || "—";
    return `${converted(value, unit).toLocaleString("ja-JP", { maximumFractionDigits: unit === "MHz" ? 0 : 1 })}${unit ? " " + unit : ""}`;
}
function timestamp(value) {
    return value
        ? new Date(value).toLocaleString("ja-JP", { hour12: false })
        : "—";
}
function current(category = "") {
    if (connection !== "CONNECTED")
        return null;
    if (category === "node")
        return nodeSample &&
            Date.now() - Date.parse(nodeSample.received_at) <
                status.stale_after_seconds * 1000
            ? nodeSample
            : null;
    return status.state === "ONLINE" && !stale ? sample : null;
}
class APIError extends Error {
    code;
    constructor(message, code = "") {
        super(message);
        this.code = code;
    }
}
async function api(path, method = "GET", body) {
    const response = await fetch("/api/v1" + path, {
        method,
        credentials: "same-origin",
        signal: AbortSignal.timeout(10000),
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = await response.json();
    if (!response.ok) {
        if (response.status === 401 &&
            data.code === "authentication" &&
            path !== "/auth/login") {
            authenticated = false;
            socket?.close();
            login();
        }
        throw new APIError(data.message || "要求に失敗しました", data.code);
    }
    return data;
}
function login(message = "") {
    generation++;
    clearTimeout(retryTimer);
    overlay.replaceChildren();
    const box = element("main", "login");
    box.append(element("div", "brand", "HOMENODE"), element("h1", "", "自分のPCを、ここから。"), element("p", "", "Tailnetに接続した端末からログインしてください。"));
    const form = element("form");
    const label = element("label", "", "管理者パスワード");
    const password = element("input");
    password.type = "password";
    password.autocomplete = "current-password";
    password.required = true;
    password.maxLength = 1024;
    label.append(password);
    const error = element("p", "notice", message);
    error.setAttribute("role", "alert");
    const submit = element("button", "primary", "ログイン");
    submit.type = "submit";
    form.append(label, error, submit);
    form.onsubmit = async (e) => {
        e.preventDefault();
        submit.disabled = true;
        error.textContent = "";
        try {
            const session = await api("/auth/login", "POST", {
                password: password.value,
            });
            password.value = "";
            csrf = session.csrf_token;
            authenticated = true;
            connection = "接続中";
            render();
            await refresh();
            connect();
        }
        catch (e) {
            error.textContent = e.message;
        }
        finally {
            submit.disabled = false;
        }
    };
    const legal = element("p", "footer", "HomeNode © 2026 tas0dev · 無保証 · ");
    const license = element("a", "", "GNU GPL v3");
    license.href = "/license.txt";
    license.target = "_blank";
    license.rel = "noopener";
    legal.append(license);
    box.append(form, legal);
    app.replaceChildren(box);
}
async function refresh() {
    if (!authenticated)
        return;
    if (fetching) {
        pending = true;
        return;
    }
    fetching = true;
    const version = generation;
    const query = new URLSearchParams({
        page: String(page),
        level: logLevel,
        component: logComponent,
    });
    try {
        const [st, metrics, history, node, sun, records] = await Promise.all([
            api("/system/status"),
            api("/system/metrics"),
            api("/system/metrics/history"),
            api("/node/metrics"),
            api("/sunshine/status"),
            api("/logs?" + query),
        ]);
        if (version !== generation || !authenticated)
            return;
        status = st;
        sample = metrics.sample;
        stale = metrics.stale;
        samples = history;
        nodeSample = node.sample;
        nodeHistory = node.history;
        sunshine = sun;
        logs = records.items;
        moreLogs = records.has_more;
        lastSync = Date.now();
        if (socket?.readyState === WebSocket.OPEN)
            connection = "CONNECTED";
        render();
    }
    catch (e) {
        if (authenticated) {
            connection = "Piとの通信が失われました";
            render();
        }
    }
    finally {
        fetching = false;
        if (pending) {
            pending = false;
            void refresh();
        }
    }
}
function appendSample(history, value) {
    const cutoff = Date.now() - 120000;
    const kept = history.filter((s) => Date.parse(s.received_at || s.timestamp) >= cutoff);
    if (value && !kept.some((s) => s.timestamp === value.timestamp))
        kept.push(value);
    return kept.slice(-120);
}
async function refreshLogs() {
    if (!authenticated)
        return;
    if (loadingLogs) {
        pendingLogs = true;
        return;
    }
    loadingLogs = true;
    const version = generation;
    try {
        const query = new URLSearchParams({
            page: String(page),
            level: logLevel,
            component: logComponent,
        });
        const data = await api("/logs?" + query);
        if (authenticated && version === generation) {
            logs = data.items;
            moreLogs = data.has_more;
            render();
        }
    }
    catch {
    }
    finally {
        loadingLogs = false;
        if (pendingLogs) {
            pendingLogs = false;
            void refreshLogs();
        }
    }
}
function connect() {
    if (!authenticated)
        return;
    clearTimeout(retryTimer);
    socket?.close();
    const ws = new WebSocket(`${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/v1/events`);
    socket = ws;
    ws.onopen = () => {
        if (socket !== ws)
            return;
        reconnect = 1000;
        void refresh();
    };
    ws.onmessage = (event) => {
        if (socket !== ws || !authenticated)
            return;
        try {
            const message = JSON.parse(event.data);
            if (message.type !== "snapshot") {
                void refresh();
                return;
            }
            status = message.status;
            sample = message.metrics.sample;
            stale = message.metrics.stale;
            nodeSample = message.node;
            sunshine = message.sunshine;
            samples = appendSample(samples, sample);
            nodeHistory = appendSample(nodeHistory, nodeSample);
            lastSync = Date.now();
            connection = "CONNECTED";
            if (message.log_marker !== logMarker) {
                logMarker = message.log_marker;
                void refreshLogs();
            }
            render();
        }
        catch {
            ws.close();
        }
    };
    ws.onerror = () => ws.close();
    ws.onclose = () => {
        if (socket !== ws || !authenticated)
            return;
        connection = "Piとの通信が失われました。再接続中です。";
        render();
        retryTimer = setTimeout(async () => {
            try {
                await api("/auth/session");
                connect();
            }
            catch {
                if (authenticated)
                    connect();
            }
        }, reconnect + Math.random() * 300);
        reconnect = Math.min(reconnect * 2, 30000);
    };
}
function render() {
    if (!authenticated)
        return;
    if (menuState && (status.state !== menuState || connection !== "CONNECTED"))
        closeMenu();
    const focus = document.activeElement?.dataset.focus;
    const scroll = window.scrollY;
    const anchor = Array.from(app.querySelectorAll(".log-entry")).find((e) => e.getBoundingClientRect().bottom > 0);
    const anchorKey = anchor?.dataset.logKey;
    const anchorTop = anchor?.getBoundingClientRect().top || 0;
    const path = location.pathname;
    const overview = path === "/";
    const shell = element("div", "shell" + (overview ? " overview" : ""));
    const topline = element("div", "topline");
    topline.append(element("span", "brand", "HOMENODE"), button("ログアウト", async () => {
        try {
            await api("/auth/logout", "POST", {});
            authenticated = false;
            socket?.close();
            login();
        }
        catch (e) {
            feedback = e.message;
            render();
        }
    }, "text-button"));
    const header = element("header", "system-header");
    header.append(element("h1", "", status.name), element("span", "state mono", status.state));
    const banner = element("div", "connection", connection === "CONNECTED" ? "" : connection);
    banner.setAttribute("role", "status");
    shell.append(topline, header, banner);
    const main = element("main");
    main.id = "main";
    if (overview)
        renderOverview(main);
    else if (path === "/metrics")
        renderMetrics(main);
    else if (path.startsWith("/metrics/") && categories[path.split("/")[2]])
        renderDetail(main, path.split("/")[2]);
    else if (path === "/logs")
        renderLogs(main);
    else
        main.append(element("p", "notice", "ページが見つかりません。"), link("Overviewへ戻る", "/"));
    shell.append(main);
    const nav = element("nav", "nav");
    nav.setAttribute("aria-label", "メインナビゲーション");
    const inner = element("div", "nav-inner");
    for (const [name, route] of [
        ["Overview", "/"],
        ["Metrics", "/metrics"],
        ["Logs", "/logs"],
    ]) {
        const a = link(name, route);
        if (route === "/" ? overview : path.startsWith(route))
            a.setAttribute("aria-current", "page");
        inner.append(a);
    }
    nav.append(inner);
    app.replaceChildren(shell, nav);
    if (overview)
        renderPower();
    if (focus)
        Array.from(app.querySelectorAll("[data-focus]"))
            .find((e) => e.dataset.focus === focus)
            ?.focus({ preventScroll: true });
    const restoredAnchor = anchorKey
        ? Array.from(app.querySelectorAll(".log-entry")).find((e) => e.dataset.logKey === anchorKey)
        : undefined;
    window.scrollTo(0, scroll +
        (restoredAnchor
            ? restoredAnchor.getBoundingClientRect().top - anchorTop
            : 0));
}
function missingNotice() {
    if (connection !== "CONNECTED")
        return "Piとの接続を確認できないため、現在値を表示していません。";
    if (status.state === "OFFLINE")
        return "Agentに接続していません。OFFLINEは物理的な電源OFFを断定する表示ではありません。";
    if (!current())
        return `更新停止 · 最終更新 ${timestamp(sample?.timestamp)}。Agentの接続または電源遷移を確認してください。`;
    return "";
}
function renderOverview(main) {
    const notice = element("p", "notice", missingNotice());
    notice.setAttribute("role", "status");
    main.append(notice);
    for (const key of ["cpu", "gpu"]) {
        const section = element("section", "section");
        const head = element("div", "section-head");
        head.append(link(key.toUpperCase(), "/metrics/" + key, "label"));
        const v = at(current(), `${key}.usage_percent`);
        const number = element("div", "value", typeof v === "number" ? format(v) : "—");
        number.append(element("span", "unit", "%"));
        head.append(number);
        section.append(head, chart("overview-" + key, samples, [{ key: `${key}.usage_percent`, name: "使用率", unit: "%" }], false));
        main.append(section);
    }
    const memory = element("section", "section");
    const mr = element("div", "row");
    const used = at(current(), "memory.used_bytes");
    mr.append(link("Memory", "/metrics/memory", "label"), element("span", "", `${format(used, "GiB")} / ${format(at(current(), "memory.total_bytes"), "GiB")} · ${format(at(current(), "memory.usage_percent"), "%")}`));
    memory.append(mr);
    main.append(memory);
    const sun = element("section", "section");
    const sr = element("div", "row");
    sr.append(element("h2", "label", "Sunshine"), element("span", "mono", current() ? sunshine.state : "UNKNOWN"));
    sun.append(sr);
    const details = element("details", "sunshine-details");
    details.open = sunshineExpanded;
    details.ontoggle = () => {
        sunshineExpanded = details.open;
    };
    const summary = element("summary", "", "接続先情報");
    details.append(summary);
    for (const [name, address] of [
        ["LAN", sunshine.lan_address],
        ["Tailscale", sunshine.tailnet_address],
    ]) {
        const row = element("div", "connection-address");
        row.append(element("span", "mono", `${name} · ${address || "—"}`));
        const copy = button("コピー", async () => {
            try {
                await navigator.clipboard.writeText(address);
                copy.textContent = "コピー済み";
            }
            catch {
                copy.textContent = "コピー失敗";
            }
        });
        copy.disabled = !address;
        row.append(copy);
        details.append(row);
    }
    details.append(element("p", "muted", "Moonlightに上記の接続先を登録してください。READYはSunshineのプロセスと指定ポートの待受を確認した状態です。Moonlightからの接続成功を保証するものではありません。"));
    sun.append(details);
    main.append(sun);
}
function renderMetrics(main) {
    const title = element("div", "page-title");
    title.append(element("h2", "", "Metrics"));
    main.append(title);
    for (const [key, category] of Object.entries(categories)) {
        const a = link("", "/metrics/" + key, "metric-link");
        const first = category.series[0];
        const s = current(key);
        a.append(element("span", "", category.name), element("span", "", format(at(s, first.key), first.unit) + " ›"));
        main.append(a);
    }
}
function getSeries(key) {
    if (!selectedSeries.has(key))
        selectedSeries.set(key, new Set(categories[key].series.slice(0, 2).map((s) => s.key)));
    return selectedSeries.get(key);
}
function renderDetail(main, key) {
    const category = categories[key];
    const latest = current(key);
    const title = element("div", "page-title");
    title.append(link("‹ 戻る", "/metrics", "back"), element("h2", "", category.name));
    main.append(title);
    if (!latest)
        main.append(element("p", "notice", key === "node"
            ? "更新停止 · 最終更新 " + timestamp(nodeSample?.timestamp)
            : missingNotice()));
    const chosen = getSeries(key);
    const active = category.series.filter((s) => chosen.has(s.key));
    const first = active[0] || category.series[0];
    main.append(element("div", "value", format(at(latest, first.key), first.unit)));
    const section = element("section", "section");
    const history = key === "node" ? nodeHistory : samples;
    section.append(chart(key, history, active, true));
    const legend = element("div", "legend");
    for (const [index, series] of category.series.entries()) {
        const b = button("", () => {
            const next = new Set(chosen);
            if (next.has(series.key))
                next.delete(series.key);
            else
                next.add(series.key);
            const units = new Set(category.series.filter((s) => next.has(s.key)).map((s) => s.unit));
            const msg = document.querySelector("#axis-message");
            if (units.size > 2) {
                if (msg)
                    msg.textContent =
                        "最大2種類の単位を表示できます。既存の系列を非表示にしてください。";
                return;
            }
            selectedSeries.set(key, next);
            render();
        });
        b.dataset.focus = series.key;
        b.setAttribute("aria-pressed", String(chosen.has(series.key)));
        const swatch = element("span", "swatch swatch-" + (active.findIndex((s) => s.key === series.key) % 3));
        swatch.setAttribute("aria-hidden", "true");
        b.append(swatch, document.createTextNode(series.name + (at(latest, series.key) === null ? " · 取得不可" : "")));
        legend.append(b);
    }
    const msg = element("p", "notice");
    msg.id = "axis-message";
    msg.setAttribute("role", "status");
    section.append(legend, msg);
    main.append(section);
    const values = element("div", "values detail-values");
    for (const series of category.series) {
        const row = element("div", "row");
        row.append(element("span", "label", series.name), element("span", "", format(at(latest, series.key), series.unit)));
        values.append(row);
    }
    for (const info of category.info) {
        const row = element("div", "row");
        const labels = {
            "cpu.model": "モデル",
            "gpu.model": "モデル",
            "network.interface": "インターフェース",
            "network.ipv4": "IPv4",
            wifi: "Wi-Fi",
            uptime_seconds: "稼働時間",
        };
        const value = at(latest, info);
        row.append(element("span", "label", labels[info]), element("span", "small", info === "uptime_seconds" && typeof value === "number"
            ? `${Math.floor(value / 86400)}日 ${Math.floor((value % 86400) / 3600)}時間 ${Math.floor((value % 3600) / 60)}分`
            : format(value)));
        values.append(row);
    }
    main.append(values);
}
function chart(key, history, series, detail) {
    const wrapper = element("div");
    const width = Math.max(288, Math.min(896, document.documentElement.clientWidth -
        (document.documentElement.clientWidth >= 640 ? 64 : 32)));
    const height = detail ? 240 : 120;
    const left = detail ? 44 : 30, right = width - (detail ? 52 : 12), top = 18, bottom = height - 30;
    const now = Date.now(), start = now - 60000;
    const data = history
        .filter((s) => Date.parse(s.timestamp) >= start && Date.parse(s.timestamp) <= now)
        .sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp));
    const units = [...new Set(series.map((s) => s.unit))];
    const svgNS = "http://www.w3.org/2000/svg";
    function svgElement(tag, attrs, text = "") {
        const e = document.createElementNS(svgNS, tag);
        for (const [k, v] of Object.entries(attrs))
            e.setAttribute(k, String(v));
        if (text)
            e.textContent = text;
        return e;
    }
    const svg = svgElement("svg", {
        viewBox: `0 0 ${width} ${height}`,
        class: "chart",
        role: "img",
        "aria-label": "直近60秒のグラフ。欠損区間は接続していません。",
        tabindex: detail ? "0" : "-1",
    });
    svg.setAttribute("data-focus", "chart-" + key);
    const bounds = new Map();
    for (const unit of units) {
        const values = [];
        for (const s of data)
            for (const line of series.filter((l) => l.unit === unit)) {
                const v = at(s, line.key);
                if (typeof v === "number")
                    values.push(converted(v, unit));
            }
        const high = unit === "%" ? 100 : Math.max(1, ...values) * 1.15;
        bounds.set(unit, high);
    }
    if (!units.length)
        bounds.set("%", 100);
    const axes = units.length ? units : ["%"];
    for (let step = 0; step < 3; step++) {
        const y = top + ((bottom - top) * step) / 2;
        svg.append(svgElement("line", { x1: left, x2: right, y1: y, y2: y, class: "grid" }));
        for (const [index, unit] of axes.entries()) {
            const value = bounds.get(unit) * (1 - step / 2);
            svg.append(svgElement("text", {
                x: index === 0 ? left - 7 : right + 7,
                y: y + 4,
                "text-anchor": index === 0 ? "end" : "start",
            }, format(value)));
        }
    }
    for (const [index, unit] of axes.entries())
        svg.append(svgElement("text", {
            x: index === 0 ? left : right,
            y: 10,
            "text-anchor": index === 0 ? "start" : "end",
        }, unit));
    svg.append(svgElement("text", { x: left, y: height - 5 }, "−60秒"), svgElement("text", { x: right, y: height - 5, "text-anchor": "end" }, "現在"));
    const xFor = (t) => left + ((t - start) / 60000) * (right - left);
    for (const [index, line] of series.entries()) {
        let points = [], previous = 0;
        const flush = () => {
            if (points.length > 1)
                svg.append(svgElement("polyline", {
                    points: points.join(" "),
                    class: "line-" + (index % 4),
                }));
            else if (points.length === 1) {
                const [x, y] = points[0].split(",");
                svg.append(svgElement("circle", { cx: x, cy: y, r: 1.5, fill: "#b8b8b8" }));
            }
            points = [];
        };
        for (const s of data) {
            const t = Date.parse(s.timestamp), value = at(s, line.key);
            if (value === null ||
                typeof value !== "number" ||
                (previous &&
                    t - previous > 2500 * (status.telemetry_interval_seconds || 1)))
                flush();
            if (typeof value === "number")
                points.push(`${xFor(t)},${bottom - (converted(value, line.unit) / bounds.get(line.unit)) * (bottom - top)}`);
            previous = t;
        }
        flush();
    }
    wrapper.append(svg);
    if (!detail)
        return wrapper;
    const selection = element("div", "selection");
    selection.setAttribute("aria-live", "polite");
    let guide = null;
    function pick(time) {
        if (!data.length) {
            selection.textContent = "取得データがありません。";
            return;
        }
        let closest = data[0];
        for (const s of data)
            if (Math.abs(Date.parse(s.timestamp) - time) <
                Math.abs(Date.parse(closest.timestamp) - time))
                closest = s;
        const t = Date.parse(closest.timestamp);
        selectedTime.set(key, t);
        guide?.remove();
        guide = svgElement("line", {
            x1: xFor(t),
            x2: xFor(t),
            y1: top,
            y2: bottom,
            class: "guide",
        });
        svg.append(guide);
        selection.replaceChildren(element("span", "mono", timestamp(closest.timestamp)));
        for (const line of series) {
            const row = element("div", "row");
            row.append(element("span", "label", line.name), element("span", "", format(at(closest, line.key), line.unit)));
            selection.append(row);
        }
    }
    svg.addEventListener("pointerdown", (e) => {
        const event = e;
        const rect = svg.getBoundingClientRect();
        const x = ((event.clientX - rect.left) / rect.width) * width;
        pick(start + Math.max(0, Math.min(1, (x - left) / (right - left))) * 60000);
    });
    svg.addEventListener("keydown", (e) => {
        const event = e;
        if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key))
            return;
        event.preventDefault();
        const time = selectedTime.get(key) ?? now;
        const index = data.findIndex((s) => Date.parse(s.timestamp) >= time);
        const next = event.key === "Home"
            ? 0
            : event.key === "End"
                ? data.length - 1
                : Math.max(0, Math.min(data.length - 1, (index < 0 ? data.length - 1 : index) +
                    (event.key === "ArrowRight" ? 1 : -1)));
        if (data[next])
            pick(Date.parse(data[next].timestamp));
    });
    if (selectedTime.has(key) && selectedTime.get(key) >= start)
        pick(selectedTime.get(key));
    else
        selection.textContent =
            "グラフをタップすると実測値を確認できます。キーボードでは左右キーで選択します。";
    wrapper.append(selection);
    return wrapper;
}
function renderLogs(main) {
    const heading = element("div", "page-title");
    heading.append(element("h2", "", "Logs"));
    main.append(heading);
    const filters = element("div", "filters");
    for (const [name, values, selected] of [
        ["重要度", ["ALL", "DEBUG", "INFO", "WARN", "ERROR"], logLevel],
        ["発生元", ["ALL", "SERVER", "AGENT", "SUNSHINE", "NODE"], logComponent],
    ]) {
        const label = element("label", "label", name);
        const select = element("select");
        select.dataset.focus = name;
        for (const value of values) {
            const option = element("option", "", value);
            option.value = value === "ALL" ? "" : value;
            option.selected = option.value === selected;
            select.append(option);
        }
        select.onchange = () => {
            if (name === "重要度")
                logLevel = select.value;
            else
                logComponent = select.value;
            page = 1;
            generation++;
            void refresh();
        };
        label.append(select);
        filters.append(label);
    }
    main.append(filters);
    if (!logs.length)
        main.append(element("p", "notice", "ログはありません。"));
    for (const log of logs) {
        const key = log.timestamp + log.event_id;
        const item = element("article", "log-entry");
        item.dataset.logKey = key;
        const b = button("", () => {
            expandedLog = expandedLog === key ? "" : key;
            render();
        });
        b.dataset.focus = key;
        b.setAttribute("aria-expanded", String(expandedLog === key));
        b.append(element("span", "mono", new Date(log.timestamp).toLocaleTimeString("ja-JP", { hour12: false })), element("span", "mono", `${log.component} / ${log.level}`), element("span", "log-id", log.event_id));
        item.append(b);
        if (expandedLog === key) {
            const detail = element("div", "log-detail");
            detail.append(element("p", "mono", log.timestamp), element("p", "", log.message), element("pre", "", JSON.stringify(log.fields, null, 2)));
            item.append(detail);
        }
        main.append(item);
    }
    const pagination = element("div", "pagination");
    const previous = button("前へ", () => {
        page--;
        generation++;
        void refresh();
    });
    previous.disabled = page <= 1;
    const next = button("次へ", () => {
        page++;
        generation++;
        void refresh();
    });
    next.disabled = !moreLogs;
    pagination.append(previous, element("span", "small", String(page)), next);
    main.append(pagination);
}
function operationText(op) {
    if (!op)
        return "";
    const names = {
        on: "Power On",
        shutdown: "Shutdown",
        restart: "Restart",
    };
    const states = {
        accepted: "Server受付済み · 状態遷移を待機しています",
        agent_accepted: "Agent受付済み · 状態遷移を待機しています",
        completed: "Agentの起動・復帰を確認しました",
        disconnected: "Agentの切断を確認しました。物理的な電源断は未確認です",
        failed: "失敗 · " +
            (op.reason === "operation timed out"
                ? "確認がタイムアウトしました"
                : op.reason || "要求を完了できませんでした"),
    };
    return `${names[op.operation] || op.operation} · ${states[op.status] || op.status}`;
}
function renderPower() {
    const bar = element("div", "power-bar");
    const inner = element("div", "power-inner");
    const progress = element("div", "power-feedback", requestBusy ? "要求中" : feedback || operationText(status.operation));
    progress.setAttribute("role", "status");
    inner.append(progress);
    const b = button("Power", openMenu, "primary");
    b.dataset.focus = "power";
    b.setAttribute("aria-haspopup", "menu");
    b.disabled =
        requestBusy ||
            connection !== "CONNECTED" ||
            !["ONLINE", "OFFLINE"].includes(status.state);
    if (status.state === "UNREACHABLE")
        b.textContent = "UNREACHABLE · Agentの接続を確認してください";
    else if (["STARTING", "SHUTTING_DOWN", "RESTARTING"].includes(status.state))
        b.textContent = status.state + " · 実行中";
    inner.append(b);
    bar.append(inner);
    app.append(bar);
}
function closeMenu() {
    if (!overlay.querySelector("dialog")) {
        menuState = "";
        overlay.replaceChildren();
    }
}
function openMenu() {
    if (requestBusy || connection !== "CONNECTED")
        return;
    menuState = status.state;
    const shade = button("", closeMenu, "menu-shade");
    shade.setAttribute("aria-label", "電源メニューを閉じる");
    shade.tabIndex = -1;
    const menu = element("div", "menu");
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-label", "電源操作");
    const operations = status.state === "OFFLINE"
        ? [["on", "Power On"]]
        : status.state === "ONLINE"
            ? [
                ["shutdown", "Shutdown"],
                ["restart", "Restart"],
            ]
            : [];
    for (const [operation, name] of operations) {
        const b = button(name, () => confirmPower(operation, name));
        b.setAttribute("role", "menuitem");
        menu.append(b);
    }
    menu.onkeydown = (e) => {
        const items = Array.from(menu.querySelectorAll("button"));
        const index = items.indexOf(document.activeElement);
        if (e.key === "Escape") {
            closeMenu();
            document.querySelector("[data-focus=power]")?.focus();
        }
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            items[(index + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus();
        }
        if (e.key === "Tab")
            closeMenu();
    };
    overlay.replaceChildren(shade, menu);
    menu.querySelector("button")?.focus();
}
function confirmPower(operation, name) {
    menuState = "";
    const dialog = element("dialog");
    const form = element("form");
    const title = element("h2", "", `${name}を実行しますか？`);
    title.id = "power-title";
    dialog.setAttribute("aria-labelledby", title.id);
    const impact = operation === "on"
        ? "Wake-on-LANを送信し、Agentの起動を待ちます。送信成功だけではPCの起動を確認できません。"
        : "Windowsに正常な" +
            (operation === "restart" ? "再起動" : "シャットダウン") +
            "を要求します。未保存の作業が失われる可能性があります。";
    const label = element("label", "label", "管理者パスワードで再認証");
    const password = element("input");
    password.type = "password";
    password.autocomplete = "current-password";
    password.required = true;
    password.maxLength = 1024;
    label.append(password);
    const error = element("p", "notice");
    error.setAttribute("role", "alert");
    const actions = element("div", "actions");
    const cancel = button("キャンセル", () => dialog.close());
    const execute = element("button", "primary", "実行");
    execute.type = "submit";
    actions.append(cancel, execute);
    form.append(title, element("p", "", `対象: ${status.name}`), element("p", "", impact), label, error, actions);
    dialog.append(form);
    dialog.addEventListener("cancel", (e) => {
        if (requestBusy)
            e.preventDefault();
    });
    dialog.addEventListener("close", () => {
        overlay.replaceChildren();
        document.querySelector("[data-focus=power]")?.focus();
    });
    form.onsubmit = async (e) => {
        e.preventDefault();
        if (requestBusy)
            return;
        requestBusy = true;
        execute.disabled = true;
        cancel.disabled = true;
        execute.textContent = "要求中";
        error.textContent = "";
        try {
            const result = await api("/system/power/" + operation, "POST", { password: password.value });
            password.value = "";
            feedback = `${name} · Server受付済み`;
            dialog.close();
            await refresh();
            feedback = "";
        }
        catch (e) {
            error.textContent = "失敗 · " + e.message;
        }
        finally {
            requestBusy = false;
            execute.disabled = false;
            cancel.disabled = false;
            execute.textContent = "実行";
            render();
        }
    };
    overlay.replaceChildren(dialog);
    dialog.showModal();
    password.focus();
}
setInterval(() => {
    if (!authenticated)
        return;
    const before = stale;
    if (sample &&
        Date.now() - Date.parse(sample.received_at) >
            status.stale_after_seconds * 1000)
        stale = true;
    if (lastSync && Date.now() - lastSync > status.stale_after_seconds * 1000) {
        connection = "Piからの更新が停止しています。再接続中です。";
        if (socket?.readyState === WebSocket.OPEN)
            socket.close();
        else if (!socket)
            connect();
    }
    if (stale !== before || connection !== "CONNECTED")
        render();
}, 1000);
window.addEventListener("online", () => {
    if (authenticated)
        connect();
});
window.addEventListener("offline", () => {
    connection =
        "Piとの通信が失われました。端末のネットワークを確認してください。";
    if (authenticated)
        render();
});
(async () => {
    try {
        const session = await api("/auth/session");
        csrf = session.csrf_token;
        authenticated = true;
        render();
        await refresh();
        connect();
    }
    catch {
        login();
    }
})();
export {};
