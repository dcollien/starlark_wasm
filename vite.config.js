import { defineConfig } from 'vite';
import dts from 'vite-plugin-dts'
import path from 'path';

export default defineConfig({
  build: {
    lib: {
      entry: path.resolve(__dirname, 'src/index.ts'),
      name: 'Starlark (wasm)',
      fileName: (format) => `starlark.${format}.js`,
      formats: ['es', 'cjs', 'umd']
    },
    rollupOptions: {
      // Make sure to externalize deps that shouldn't be bundled
      // into your library
      external: [],
      output: {
        // Provide global variables to use in the UMD build
        // for externalized deps
        globals: {}
      }
    },
    target: 'esnext',
  },
  plugins: [dts()],
  assetsInclude: ['**/*.wasm'],
});
