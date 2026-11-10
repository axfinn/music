import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// 前端构建配置。
//
// 设计要点：
//   - Element Plus 组件按需自动引入（unplugin-vue-components + ElementPlusResolver），
//     业务组件直接写 <el-button> 不必逐个 import，样式也随组件一起注入。
//   - 产物输出到 dist/，由 Go 后端直接托管（见 main.go 的静态文件分支）。
//   - dev 模式把 /api 代理到本地后端，前端代码里一律用相对路径 /api/...，
//     生产环境同源，不需要任何 base path 配置。
export default defineConfig({
  plugins: [
    vue(),
    Components({
      resolvers: [ElementPlusResolver({ importStyle: 'css' })],
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    // 资源文件名带 hash，便于长期缓存。
    assetsDir: 'assets',
  },
})
