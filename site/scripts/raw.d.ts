// content.ts type-imports icons.ts, which uses Vite's `?raw` imports.
declare module '*?raw' {
  const source: string;
  export default source;
}
