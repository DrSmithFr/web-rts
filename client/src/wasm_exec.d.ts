// Definitions for the Go toolchain bridge script loaded via <script src="/wasm_exec.js">.
declare class Go {
  argv: string[];
  env: Record<string, string>;
  importObject: WebAssembly.Imports;
  exited: boolean;
  exit(code: number): void;
  run(instance: WebAssembly.Instance): Promise<void>;
}
