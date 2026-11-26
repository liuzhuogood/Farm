const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const ts = require('typescript');
const module_ = { exports: {} };
const source = ts.transpileModule(fs.readFileSync(path.join(__dirname, 'doubleTap.ts'), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;
vm.runInNewContext(source, { module: module_, exports: module_.exports });
const { createDoubleTapDetector } = module_.exports;

let fired = 0;
const detector = createDoubleTapDetector(400, () => fired++);

assert.equal(detector.tap(1000), false, '单击只记录时间');
assert.equal(fired, 0, '单击不触发');
assert.equal(detector.tap(1300), true, '窗口期内第二次点击触发');
assert.equal(fired, 1);

assert.equal(detector.tap(1400), false, '触发后重新计数，三击不再算双击');
assert.equal(detector.tap(1900), false, '超出窗口期不触发');
assert.equal(detector.tap(2000), true, '重新在窗口期内点击仍可触发');
assert.equal(fired, 2);

detector.tap(3000);
detector.reset();
assert.equal(detector.tap(3100), false, 'reset 后重新开始计时');

console.log('PASS: double tap detector window, reset after fire and explicit reset');
