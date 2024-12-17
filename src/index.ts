import {
  StarlarkInterface,
  StarlarkCompatibleDict,
  StarlarkCompatibleValue,
  StarlarkConfig,
  StarlarkGlobal,
  Loader,
  PrintFn,
} from "./types.js";

import "./wasm_exec.js";

const starlark: StarlarkGlobal = {
  load: async (filename, executionId) => {
    if (!starlark._executions[executionId]) {
      throw new Error("Unable to load. No execution found: " + executionId);
    }
    return await starlark._executions[executionId].load(filename, executionId);
  },
  print: (message, executionId) => {
    if (!starlark._executions[executionId]) {
      throw new Error("Unable to print. No execution found: " + executionId);
    }
    starlark._executions[executionId].print(message, executionId);
  },
  _executions: {},
};

const init = async (wasmUrl: string) => {
  const global = window as any;
  global.starlark = starlark;

  const go = new global.Go();
  const wasmModule = await WebAssembly.instantiateStreaming(
    fetch(wasmUrl),
    go.importObject
  );
  go.run(wasmModule.instance);
};

const defaultLoad = async (_filename: string, _executionId: string) => {
  throw new Error("No loader provided");
};

const defaultPrint = (message: string, _executionId: string) => {
  console.log(message);
};

export class Starlark implements StarlarkInterface {
  print: PrintFn;
  load: Loader;
  maxExecutionTime?: StarlarkConfig["maxExecutionTime"];

  static async init(wasm: string) {
    await init(wasm);
  }

  constructor(config: StarlarkConfig) {
    this.print = config.print || defaultPrint;
    this.load = config.load || defaultLoad;
    this.maxExecutionTime = config.maxExecutionTime;
  }

  async run(
    filename: string,
    functionName: string = "main",
    args?: StarlarkCompatibleValue[],
    kwargs?: StarlarkCompatibleDict,
    maxExecutionTime?: number
  ): Promise<StarlarkCompatibleValue> {
    if (!starlark.wasm_runner) {
      throw new Error("Starlark not initialized");
    }

    const executionId = Math.random().toString().slice(2);

    if (maxExecutionTime === undefined) {
      maxExecutionTime = this.maxExecutionTime;
    }

    starlark._executions[executionId] = this;

    const returnValue = await starlark.wasm_runner(
      executionId,
      filename,
      functionName || "main",
      args || [],
      kwargs || {},
      maxExecutionTime || 0
    );

    delete starlark._executions[executionId];

    return returnValue;
  }
}
