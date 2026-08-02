/// <reference types="vite/client" />

declare module '*.erd?raw' {
  const content: string
  export default content
}
