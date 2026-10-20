import ts from "typescript";
import { mkdir, readFile, writeFile, copyFile } from "node:fs/promises";
await mkdir(new URL("./dist/", import.meta.url), { recursive: true });
const source = await readFile(new URL("./src/app.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
	compilerOptions: {
		target: ts.ScriptTarget.ES2022,
		module: ts.ModuleKind.ESNext,
		removeComments: true,
	},
});
await writeFile(new URL("./dist/app.js", import.meta.url), output.outputText);
for (const name of ["index.html", "style.css"]) {
	await copyFile(
		new URL(`./src/${name}`, import.meta.url),
		new URL(`./dist/${name}`, import.meta.url),
	);
}
await copyFile(
	new URL("../license", import.meta.url),
	new URL("./dist/license.txt", import.meta.url),
);
