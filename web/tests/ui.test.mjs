import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
const { chromium } = createRequire(import.meta.url)("playwright");

const stamp = () => new Date().toISOString();
const makeSample = () => ({
	timestamp: stamp(),
	received_at: stamp(),
	cpu: {
		usage_percent: 36.2,
		temperature_c: 53,
		clock_mhz: 4800,
		model: "AMD Ryzen 7 9700X",
	},
	gpu: {
		usage_percent: 67.8,
		temperature_c: 62,
		clock_mhz: 2500,
		vram_used_bytes: 4 * 2 ** 30,
		vram_total_bytes: 12 * 2 ** 30,
		power_w: 120,
		model: "AMD Radeon RX 7700 XT",
	},
	memory: {
		used_bytes: 8 * 2 ** 30,
		total_bytes: 32 * 2 ** 30,
		available_bytes: 24 * 2 ** 30,
		usage_percent: 25,
	},
	network: {
		interface: "Ethernet",
		ipv4: "192.168.1.20",
		rx_bytes_per_sec: 20 * 2 ** 20,
		tx_bytes_per_sec: 2 ** 20,
		rx_total_bytes: 2 ** 30,
		tx_total_bytes: 2 ** 29,
		link_speed_bps: 1e9,
	},
	uptime_seconds: 12345,
	wifi: "CONNECTED",
});

test("HomeNode UI: navigation, charts, logs, confirmation and disconnect", async () => {
	const browser = await chromium.launch({
		headless: true,
		args: ["--no-sandbox"],
	});
	const server = createServer(async (req, res) => {
		try {
			const path =
				req.url === "/" || !req.url.includes(".")
					? "index.html"
					: req.url.slice(1);
			const body = await readFile(new URL("../dist/" + path, import.meta.url));
			res.setHeader(
				"Content-Type",
				path.endsWith(".css")
					? "text/css"
					: path.endsWith(".js")
						? "text/javascript"
						: "text/html",
			);
			res.end(body);
		} catch {
			res.writeHead(404);
			res.end();
		}
	});
	await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
	const base = "http://127.0.0.1:" + server.address().port;
	const page = await browser.newPage({ viewport: { width: 320, height: 760 } });
	let ws,
		state = "ONLINE",
		powerRequests = 0,
		loggedIn = false;
	const errors = [];
	page.on("pageerror", (e) => errors.push(e.message));
	await page.routeWebSocket("**/api/v1/events", (socket) => {
		ws = socket;
	});
	await page.route("**/api/v1/**", async (route) => {
		const url = new URL(route.request().url());
		const path = url.pathname.replace("/api/v1", "");
		let data;
		if (path === "/auth/login") {
			loggedIn = true;
			data = { csrf_token: "test-csrf" };
		} else if (!loggedIn) {
			await route.fulfill({
				status: 401,
				json: { code: "authentication", message: "ログインしてください" },
			});
			return;
		} else if (path === "/auth/session") data = { csrf_token: "test-csrf" };
		else if (path === "/system/status")
			data = {
				name: "Windows PC",
				state,
				stale: state !== "ONLINE",
				stale_after_seconds: 15,
				last_heartbeat: stamp(),
				operation: null,
			};
		else if (path === "/system/metrics")
			data = { sample: makeSample(), stale: state !== "ONLINE" };
		else if (path === "/system/metrics/history")
			data = Array.from({ length: 60 }, (_, i) => ({
				...makeSample(),
				timestamp: new Date(Date.now() - (59 - i) * 1000).toISOString(),
			}));
		else if (path === "/node/metrics")
			data = { sample: makeSample(), history: [makeSample()] };
		else if (path === "/sunshine/status")
			data = {
				state: "READY",
				lan_address: "192.168.1.20",
				tailnet_address: "desktop.test.ts.net",
			};
		else if (path === "/logs")
			data = {
				items: [
					{
						timestamp: stamp(),
						level: url.searchParams.get("level") || "INFO",
						component: url.searchParams.get("component") || "SERVER",
						event_id: "state.changed",
						message: "PCの状態が変わりました",
						fields: { to: "ONLINE" },
					},
				],
				has_more: false,
			};
		else if (path.startsWith("/system/power/")) {
			powerRequests++;
			assert.equal(route.request().headers()["x-csrf-token"], "test-csrf");
			state = "SHUTTING_DOWN";
			data = { status: "accepted", request_id: "test-operation" };
		} else data = {};
		await route.fulfill({ json: data });
	});
	try {
		await page.goto(base);
		await page
			.getByLabel("管理者パスワード", { exact: true })
			.fill("test-password-123");
		await page.getByRole("button", { name: "ログイン", exact: true }).click();
		await page.waitForFunction(() =>
			document.body.textContent.includes("36.2"),
		);
		assert.equal(
			await page.evaluate(
				() => document.documentElement.scrollWidth <= innerWidth,
			),
			true,
			"320px fits viewport",
		);
		await page.getByText("接続先情報", { exact: true }).click();
		assert.equal(await page.getByText("LAN · 192.168.1.20").count(), 1);
		await page.getByRole("link", { name: "Metrics", exact: true }).click();
		await page.locator('a[href="/metrics/cpu"]').click();
		await page.getByRole("button", { name: "クロック", exact: true }).click();
		assert.match(
			await page.locator("#axis-message").textContent(),
			/最大2種類/,
		);
		await page.getByRole("button", { name: "温度", exact: true }).click();
		await page.getByRole("button", { name: "クロック", exact: true }).click();
		assert.equal(
			await page
				.getByRole("button", { name: "クロック", exact: true })
				.getAttribute("aria-pressed"),
			"true",
		);
		await page.locator("svg").click({ position: { x: 120, y: 100 } });
		assert.match(await page.locator(".selection").textContent(), /使用率/);
		await page.locator("svg").focus();
		await page.keyboard.press("ArrowLeft");
		await page.goBack();
		assert.equal(new URL(page.url()).pathname, "/metrics");
		for (const category of ["gpu", "memory", "network", "node"]) {
			await page.locator(`a[href="/metrics/${category}"]`).click();
			assert.equal(await page.locator("svg").count(), 1);
			await page.getByRole("link", { name: "戻る" }).click();
		}
		await page.getByRole("link", { name: "Logs", exact: true }).click();
		await page.getByLabel("重要度").selectOption("ERROR");
		await page.waitForFunction(() =>
			document.body.textContent.includes("SERVER / ERROR"),
		);
		await page.getByLabel("発生元").selectOption("AGENT");
		await page.waitForFunction(() =>
			document.body.textContent.includes("AGENT / ERROR"),
		);
		await page.locator(".log-entry > button").click();
		assert.match(await page.locator(".log-detail").textContent(), /ONLINE/);
		await page.getByRole("link", { name: "Overview", exact: true }).click();
		await page.getByRole("button", { name: "Power", exact: true }).click();
		assert.equal(
			await page
				.getByRole("menuitem", { name: "Power On", exact: true })
				.count(),
			0,
		);
		await page.getByRole("menuitem", { name: "Shutdown", exact: true }).click();
		assert.match(await page.locator("dialog").textContent(), /未保存の作業/);
		await page.getByRole("button", { name: "キャンセル", exact: true }).click();
		assert.equal(powerRequests, 0);
		await page.getByRole("button", { name: "Power", exact: true }).click();
		await page.getByRole("menuitem", { name: "Shutdown", exact: true }).click();
		await page.getByLabel("管理者パスワードで再認証").fill("test-password-123");
		await page.getByRole("button", { name: "実行", exact: true }).click();
		await page.waitForFunction(() =>
			document.body.textContent.includes("SHUTTING_DOWN · 実行中"),
		);
		assert.equal(powerRequests, 1);
		assert.equal(await page.locator("[data-focus=power]").isDisabled(), true);
		state = "OFFLINE";
		ws.send(JSON.stringify({ type: "refresh" }));
		await page.waitForFunction(() =>
			document.body.textContent.includes("OFFLINEは物理的"),
		);
		assert.equal(await page.locator(".value").first().textContent(), "—%");
		state = "ONLINE";
		ws.send(JSON.stringify({ type: "refresh" }));
		await page.waitForFunction(() =>
			document.body.textContent.includes("36.2"),
		);
		ws.close();
		await page.waitForFunction(() =>
			document.body.textContent.includes("Piとの通信が失われました"),
		);
		assert.equal(await page.locator(".value").first().textContent(), "—%");
		assert.deepEqual(errors, []);
		await page.setViewportSize({ width: 1280, height: 900 });
		assert.equal(
			await page.evaluate(
				() => document.documentElement.scrollWidth <= innerWidth,
			),
			true,
		);
	} finally {
		await browser.close();
		await new Promise((resolve) => server.close(resolve));
	}
});
