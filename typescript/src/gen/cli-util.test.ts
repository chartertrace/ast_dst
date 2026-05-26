import { describe, it, expect, vi, afterEach } from "vitest";
import { writeFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { loadModel } from "./cli-util";

// die() ends the process; capture the stderr message and turn exit into a throw
// so we can assert on the guidance instead of killing the test runner.
function capture(fn: () => void): { msg: string; exited: boolean } {
  let msg = "";
  let exited = false;
  const exit = vi.spyOn(process, "exit").mockImplementation((() => {
    exited = true;
    throw new Error("__exit__");
  }) as never);
  const write = vi.spyOn(process.stderr, "write").mockImplementation(((s: string) => {
    msg += s;
    return true;
  }) as never);
  try {
    fn();
  } catch (e) {
    if ((e as Error).message !== "__exit__") throw e;
  } finally {
    exit.mockRestore();
    write.mockRestore();
  }
  return { msg, exited };
}

const tmp: string[] = [];
const tmpFile = (name: string, body: string) => {
  const p = join(tmpdir(), `astdst-${Date.now()}-${name}`);
  writeFileSync(p, body);
  tmp.push(p);
  return p;
};
afterEach(() => {
  for (const p of tmp.splice(0)) rmSync(p, { force: true });
});

describe("loadModel", () => {
  it("returns the parsed model for a valid file", () => {
    const m = loadModel("sample/model.json"); // shipped fixture
    expect(Array.isArray(m.operations)).toBe(true);
  });

  it("gives actionable guidance when the file is missing", () => {
    const { msg, exited } = capture(() => loadModel("/no/such/model.json"));
    expect(exited).toBe(true);
    expect(msg).toContain("no model at");
    expect(msg).toContain("Run the Go extractor first");
  });

  it("reports invalid JSON with the file name", () => {
    const p = tmpFile("bad.json", "{ not json");
    const { msg } = capture(() => loadModel(p));
    expect(msg).toContain("is not valid JSON");
  });

  it("rejects a well-formed file that isn't a model", () => {
    const p = tmpFile("wrong.json", JSON.stringify({ hello: "world" }));
    const { msg } = capture(() => loadModel(p));
    expect(msg).toContain(`doesn't look like a model.json`);
  });
});
