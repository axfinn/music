/**
 * 页面级依赖注入：顶层创建一次 store / playback / lyrics / upload / ui，
 * 子组件按需 inject，避免十几层 props 透传，也让组件能独立测试（注入 mock 即可）。
 */
import { inject, provide } from 'vue'

const KEY = Symbol('music-library-context')

export function provideMusicContext(ctx) {
  provide(KEY, ctx)
}

export function useMusicContext() {
  const ctx = inject(KEY, null)
  if (!ctx) throw new Error('MusicLibrary 组件必须挂在 MusicLibraryPage 内部')
  return ctx
}
