# Starlark in the Browser

A typescript library for running starlark-go in the browser using WebAssembly.

`npm install starlark-wasm`

## Example

```typescript
import { Starlark } from "starlark-wasm";
import wasmUrl from "starlark-wasm/wasm?url";

const exampleCode = `
def hello_world(name):
    print("hello " + name)
    return 42`;

// load the wasm module
Starlark.init(wasmUrl);

// instantiate a new starlark runtime, defining a load and print function
const starlark = new Starlark({
  // load gives you module loading
  load: async (filename) => {
    const files = {
      "main.star": exampleCode,
    };
    return files[filename];
  },

  // print to a console
  print: (message) => {
    console.log(message);
  },
});

const returnValue = await starlark.run(
  "main.star", // the file to run
  "hello_world", // the function to call
  ["starlark"], // the args for the function
  {}, // the kwargs for the function
  1 // maximum execution seconds before timeout
);
```

## Project Structure

- `index.html`: A demo of using this library, running starlark in the browser
- `go/`: The go code that compiles to `starlark.wasm`
- `public/`: Where `starlark.wasm` lives. Note: this is to be hosted and included as an asset in your project
- `src/`: The typescript project

## Implementation details

The WebAssembly code adds a `wasm_runner` function a `window.starlark` global. This global object also has the underlying `print` and `load` functions which `wasm_runner` calls during execution. Calling `run` on a `Starlark` instance also registers an execution thread with this global object, so that when the underlying `load` and `print` functions are called, they can invoke the respective functions on the relevant instance. This allows you to set up multiple runtimes/projects and call starlark functions on them independently, e.g. loading from different file structures or printing to different consoles.

## Building

1. Build `starlark.wasm`

   ```
   npm run build-go
   ```

2. Build the typescript project

   ```
   npm run build
   ```

3. Run the demo

   ```
   npm run dev
   ```

## Credits

Borrowed concepts and inspiration from https://github.com/HarikrishnanBalagopal/starlark-webasm
