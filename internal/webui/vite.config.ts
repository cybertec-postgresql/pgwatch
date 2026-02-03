import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tsconfigPaths from 'vite-tsconfig-paths';
import svgr from 'vite-plugin-svgr';

const handleGoTemplates = (command: string) => ({
  name: 'handle-go-templates',
  transformIndexHtml(html: string) {
    if (command === 'serve') {
      return html
        .replace(/{{if \.BasePath}}([\s\S]*?){{end}}/g, '$1')
        .replace(/{{.*?}}/g, '')
        .replace(/href="\/\/+"/g, 'href="/"')
        .replace(/([^:])\/\//g, '$1/');
    }
    return html;
  },
});

export default defineConfig(({ command }) => ({
  plugins: [
    handleGoTemplates(command),
    react(),
    tsconfigPaths(),
    svgr({
      include: '**/*.svg',
    }),
  ],
  server: {
    port: 4000,
    proxy: {
      '/source': 'http://localhost:8080',
      '/metric': 'http://localhost:8080',
      '/preset': 'http://localhost:8080',
      '/login': 'http://localhost:8080',
      '/test-connect': 'http://localhost:8080',
      '/liveness': 'http://localhost:8080',
      '/readiness': 'http://localhost:8080',
      '/log': {
        target: 'ws://localhost:8080',
        ws: true,
      },
    },
  },
  build: {
    outDir: '../webserver/build',
    emptyOutDir: true,
  },
  define: {
    'process.env': {},
  },
}));
