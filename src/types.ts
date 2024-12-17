export interface StarlarkCompatibleDict {
  [key: string]: StarlarkCompatibleValue;
}

export type StarlarkCompatibleValue =
  | StarlarkCompatibleDict
  | Array<StarlarkCompatibleValue>
  | number
  | string
  | boolean
  | null;

export type Loader = (filename: string, executionId: string) => Promise<string>;
export type PrintFn = (message: string, executionId: string) => void;

export interface StarlarkConfig {
  load?: Loader;
  print?: PrintFn;
  maxExecutionTime?: number;
}

export interface StarlarkInterface {
  print: PrintFn;
  load: Loader;
  maxExecutionTime?: number;

  run(
    filename: string,
    functionName?: string,
    args?: StarlarkCompatibleValue[],
    kwargs?: StarlarkCompatibleDict,
    maxExecutionTime?: number
  ): Promise<StarlarkCompatibleValue>;
}

export interface StarlarkGlobal {
  wasm_runner?: (
    executionId: string,
    filename: string,
    fn: string,
    args?: StarlarkCompatibleValue[],
    kwargs?: StarlarkCompatibleDict,
    maxExecutionTime?: number
  ) => StarlarkCompatibleValue;

  load?: (filename: string, executionId: string) => Promise<string>;
  print?: (message: string, executionId: string) => void;

  _executions: {
    [executionId: string]: StarlarkInterface;
  };
}
