// Copyright 2024 David Collien

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"syscall/js"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func convertToStarlarkValue(value js.Value) starlark.Value {
	switch value.Type() {
	case js.TypeBoolean:
		return starlark.Bool(value.Bool())
	case js.TypeNumber:
		floatVal := value.Float()
		if floatVal == float64(int(floatVal)) {
			return starlark.MakeInt(value.Int())
		}
		return starlark.Float(floatVal)
	case js.TypeString:
		return starlark.String(value.String())
	case js.TypeObject:
		if value.InstanceOf(js.Global().Get("Array")) {
			list := []starlark.Value{}
			length := value.Length()
			for i := 0; i < length; i++ {
				list = append(list, convertToStarlarkValue(value.Index(i)))
			}
			return starlark.NewList(list)
		} else {
			dict := starlark.NewDict(value.Length())
			keys := js.Global().Get("Object").Call("keys", value)
			length := keys.Length()
			for i := 0; i < length; i++ {
				key := keys.Index(i).String()
				dict.SetKey(starlark.String(key), convertToStarlarkValue(value.Get(key)))
			}
			return dict
		}
	default:
		return starlark.None
	}
}

func convertToJSValue(value starlark.Value) js.Value {
	switch v := value.(type) {
	case starlark.Bool:
		return js.ValueOf(bool(v))
	case starlark.Float:
		return js.ValueOf(float64(v))
	case starlark.String:
		return js.ValueOf(string(v))
	case starlark.Int:
		intVal, _ := v.Int64()
		return js.ValueOf(intVal)
	case *starlark.List:
		array := js.Global().Get("Array").New(v.Len())
		for i := 0; i < v.Len(); i++ {
			array.SetIndex(i, convertToJSValue(v.Index(i)))
		}
		return array
	case *starlark.Dict:
		obj := js.Global().Get("Object").New()
		for _, item := range v.Items() {
			key := item[0].(starlark.String)
			obj.Set(string(key), convertToJSValue(item[1]))
		}
		return obj
	default:
		return js.Null()
	}
}

func jsAwait(promise js.Value) (js.Value, error) {
	done := make(chan struct{})
	var result js.Value
	var err error

	promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		result = args[0]
		close(done)
		return nil
	}), js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		err = fmt.Errorf(args[0].String())
		close(done)
		return nil
	}))

	<-done
	return result, err
}

func loadFile(filename string, executionId string) (string, error) {
	starlarkObj := js.Global().Get("starlark")
	if starlarkObj.IsUndefined() || starlarkObj.IsNull() {
		return "", fmt.Errorf("Error: window.starlark is not defined.")
	}

	loadPromise := starlarkObj.Get("load").Invoke(filename, executionId)

	// Wait for the promise to resolve.
	result, err := jsAwait(loadPromise)
	if err != nil {
		return "", fmt.Errorf("Error: failed to load the file %q. Error: %q", filename, err)
	}

	return result.String(), nil
}

func jsPrint(msg string, executionId string) {
	starlarkObj := js.Global().Get("starlark")
	if starlarkObj.IsUndefined() || starlarkObj.IsNull() {
		fmt.Println(msg)
	}

	starlarkObj.Get("print").Invoke(msg, executionId)
}

func runStarlarkCode(executionId string, filename string, funcName string, args []starlark.Value, kwargs []starlark.Tuple) (starlark.Value, error) {
	print := func(_ *starlark.Thread, msg string) {
		jsPrint(msg, executionId)
	}

	type entry struct {
		globals starlark.StringDict
		err     error
	}
	cache := make(map[string]*entry)

	var load func(_ *starlark.Thread, module string) (starlark.StringDict, error)
	load = func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
		e, ok := cache[module]
		if e == nil {
			if ok {
				// request for package whose loading is in progress
				return nil, fmt.Errorf("cycle in load graph")
			}
			// Add a placeholder to indicate "load in progress".
			cache[module] = nil

			// Load and initialize the module in a new thread.
			data, err := loadFile(module, executionId)
			fileOptions := syntax.FileOptions{} // zero value for default behavior. TODO: add support for custom file options.

			thread := &starlark.Thread{Name: executionId + " exec " + module, Load: load, Print: print}
			globals, err := starlark.ExecFileOptions(&fileOptions, thread, module, data, nil)
			e = &entry{globals, err}

			// Update the cache.
			cache[module] = e
		}
		return e.globals, e.err
	}

	globals, err := load(nil, filename)
	if err != nil {
		err := fmt.Errorf("Error: unable to evaluate the starlark code. %q", err)
		return nil, err
	}
	starlarkFn, ok := globals[funcName]
	if !ok {
		err := fmt.Errorf("Error: the function %q is missing.", funcName)
		return nil, err
	}

	// Call the function.
	thread := &starlark.Thread{Name: executionId, Load: load, Print: print}
	returnValue, err := starlark.Call(thread, starlarkFn, args, kwargs)
	if err != nil {
		err := fmt.Errorf("Error: unable to execute the starlark code. %q", err)
		return nil, err
	}
	return returnValue, nil
}

func runStarlarkCodeWithTimeout(executionId string, filename string, funcName string, args []starlark.Value, kwargs []starlark.Tuple, maxExecutionTime int) (starlark.Value, error) {
	if maxExecutionTime <= 0 {
		return runStarlarkCode(executionId, filename, funcName, args, kwargs)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(maxExecutionTime)*time.Second)
	defer cancel()

	resultChan := make(chan struct {
		value starlark.Value
		err   error
	})

	go func() {
		value, err := runStarlarkCode(executionId, filename, funcName, args, kwargs)
		resultChan <- struct {
			value starlark.Value
			err   error
		}{value, err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("Error: execution timed out")
	case result := <-resultChan:
		return result.value, result.err
	}
}

func runStarlarkCodeJs(args []js.Value) (js.Value, error) {
	if len(args) < 3 {
		err := fmt.Errorf("Error: requires executionId, filename, and functionName as arguments.")
		return js.Null(), err
	}

	executionId := args[0].String()
	filename := args[1].String()
	funcName := args[2].String()

	jsArgs := js.Null()
	if len(args) > 3 {
		jsArgs = args[3]
	}

	jsKwargs := js.Null()
	if len(args) > 4 {
		jsKwargs = args[4]
	}

	maxExecutionTime := 0
	if len(args) > 5 {
		maxExecutionTime = args[5].Int()
	}

	starlarkArgs := []starlark.Value{}
	starlarkKwargs := []starlark.Tuple{}

	if jsArgs.Type() == js.TypeObject && jsArgs.InstanceOf(js.Global().Get("Array")) {
		for i := 0; i < jsArgs.Length(); i++ {
			starlarkArgs = append(starlarkArgs, convertToStarlarkValue(jsArgs.Index(i)))
		}
	}

	if jsKwargs.Type() == js.TypeObject {
		keys := js.Global().Get("Object").Call("keys", jsKwargs)
		for i := 0; i < keys.Length(); i++ {
			key := keys.Index(i).String()
			starlarkKwargs = append(starlarkKwargs, starlark.Tuple{starlark.String(key), convertToStarlarkValue(jsKwargs.Get(key))})
		}
	}

	returnValue, err := runStarlarkCodeWithTimeout(executionId, filename, funcName, starlarkArgs, starlarkKwargs, maxExecutionTime)

	if err != nil {
		return js.Null(), err
	} else {
		return convertToJSValue(returnValue), nil
	}
}

func jsAsyncStarlarkRunner() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, promiseArgs []js.Value) interface{} {
			resolve := promiseArgs[0]
			reject := promiseArgs[1]
			go func() {
				defer func() {
					if r := recover(); r != nil {
						reject.Invoke(r)
					}
				}()
				returnValue, err := runStarlarkCodeJs(args)
				if err != nil {
					reject.Invoke(err.Error())
				} else {
					resolve.Invoke(returnValue)
				}
			}()
			return nil
		}))
	})
}

func main() {
	starlarkObj := js.Global().Get("starlark")
	if starlarkObj.IsUndefined() || starlarkObj.IsNull() {
		starlarkObj = js.Global().Get("Object").New()
		js.Global().Set("starlark", starlarkObj)
	}
	starlarkObj.Set("wasm_runner", jsAsyncStarlarkRunner())
	<-make(chan bool)
}
