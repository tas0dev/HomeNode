import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";

async function load() {
	const source = await readFile(
		new URL("../dist/app.js", import.meta.url),
		"utf8",
	);
	const bootstrap = source.lastIndexOf("(async () => {");
	assert.ok(bootstrap > 0);
	const context = vm.createContext({
		document: { querySelector: () => ({}) },
		window: { addEventListener() {} },
		setInterval() {},
		Date,
		URLSearchParams,
		Error,
		Map,
		Set,
		Number,
	});
	vm.runInContext(
		source.slice(0, bootstrap) +
			"\nglobalThis.view = { at, format, current, missingNotice, appendSample, operationText };",
		context,
	);
	return context;
}

test("Unavailable values stay missing, while a measured zero remains zero", async () => {
	const { view } = await load();
	assert.equal(view.format(null, "%"), "—");
	assert.equal(view.format(0, "%"), "0 %");
	assert.equal(view.format(Infinity, "W"), "—");
	assert.equal(view.at(null, "cpu.usage_percent"), null);
	assert.equal(view.format(1073741824, "GiB"), "1 GiB");
});

test("Offline, stale and disconnected snapshots cannot become current readings", async () => {
	const context = await load();
	vm.runInContext(
		`sample = {cpu:{usage_percent:42}}; connection = 'CONNECTED'; status.state = 'ONLINE'; stale = false;`,
		context,
	);
	assert.equal(context.view.current().cpu.usage_percent, 42);
	vm.runInContext(`status.state = 'OFFLINE';`, context);
	assert.equal(context.view.current(), null);
	assert.match(context.view.missingNotice(), /物理的な電源OFFを断定/);
	vm.runInContext(`status.state = 'ONLINE'; stale = true;`, context);
	assert.equal(context.view.current(), null);
	assert.match(context.view.missingNotice(), /更新停止/);
	vm.runInContext(`stale = false; connection = 'DISCONNECTED';`, context);
	assert.equal(context.view.current(), null);
});

test("Live history deduplicates received snapshots and expires the missing interval", async () => {
	const { view } = await load();
	const now = new Date().toISOString();
	const old = new Date(Date.now() - 121000).toISOString();
	const current = { timestamp: now, received_at: now };
	const history = view.appendSample(
		[{ timestamp: old, received_at: old }, current],
		current,
	);
	assert.equal(history.length, 1);
	assert.equal(history[0].timestamp, now);
});

test("Acceptance is distinct from actual startup and shutdown disconnection", async () => {
	const { view } = await load();
	assert.match(
		view.operationText({ operation: "restart", status: "accepted" }),
		/Server受付済み/,
	);
	assert.match(
		view.operationText({ operation: "restart", status: "agent_accepted" }),
		/Agent受付済み/,
	);
	assert.match(
		view.operationText({ operation: "restart", status: "completed" }),
		/起動・復帰を確認/,
	);
	assert.match(
		view.operationText({ operation: "shutdown", status: "disconnected" }),
		/物理的な電源断は未確認/,
	);
});
