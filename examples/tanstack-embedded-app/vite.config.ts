import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Puerto 5174 para no chocar con el dev server del shell (5173).
// Es el mismo default que sugiere el Dev tester en /t/{slug}/dev.
export default defineConfig({
  plugins: [react()],
  server: { port: 5174 },
});
