// 屏幕常亮（Screen Wake Lock API）。
//
// 只在「页面激活 + 文档可见 + 正在播放 + 用户开启」四个条件同时成立时持锁，
// 任一条件失效立刻退还，绝不把锁漏在后台。
//
// 两个必须处理的浏览器行为：
//   1. 页面切后台时浏览器会**自动释放**锁，回到前台必须重新申请 —— 靠 visibilitychange；
//   2. 申请是异步的，期间可能已经暂停/切走/卸载 —— 用 generation 作废在途结果，
//      否则会把一把没人要的锁挂在手上（屏幕一直亮到关页面）。
//
// 不支持的浏览器（Firefox、部分 iOS 等）静默降级：supported=false，其余调用都是空操作。
// 测试可注入 navigator/document 替身，避免依赖 jsdom 里并不存在的 wakeLock。

import { onBeforeUnmount, ref } from 'vue'

/**
 * @param {{ navigator?: object, document?: object }} [env] 仅测试用依赖注入
 */
export function createWakeLock(env = {}) {
  const nav = env.navigator ?? (typeof navigator !== 'undefined' ? navigator : null)
  const doc = env.document ?? (typeof document !== 'undefined' ? document : null)

  const supported = !!(nav && nav.wakeLock && typeof nav.wakeLock.request === 'function')
  const locked = ref(false)
  let sentinel = null
  let wanted = false
  let generation = 0
  let listening = false

  async function releaseSentinel() {
    generation++ // 作废旧申请：它们回来时会自己把锁还掉
    const s = sentinel
    sentinel = null
    locked.value = false
    if (s && !s.released) {
      try { await s.release() } catch { /* 浏览器可能已先释放，忽略 */ }
    }
  }

  async function apply() {
    if (!supported) return
    if (!wanted || (doc && doc.hidden)) {
      await releaseSentinel()
      return
    }
    if (sentinel && !sentinel.released) return

    const myGen = ++generation
    try {
      const s = await nav.wakeLock.request('screen')
      // 异步窗口里状态可能已经变了：立刻退还，不持有这把过期的锁
      if (myGen !== generation || !wanted || (doc && doc.hidden)) {
        try { await s.release() } catch {}
        return
      }
      sentinel = s
      locked.value = true
      // 省电策略等场景下浏览器会主动 release，同步状态避免 UI 显示"已锁"实则没锁
      if (typeof s.addEventListener === 'function') {
        s.addEventListener('release', () => {
          if (sentinel !== s) return
          sentinel = null
          locked.value = false
          // 文档仍可见且还需要亮屏时，浏览器主动释放后补一次申请
          if (wanted && !(doc && doc.hidden)) void apply()
        })
      }
    } catch {
      locked.value = false
    }
  }

  function setWanted(next) {
    wanted = !!next
    void apply()
  }

  function onVisibility() {
    if (!doc) return
    if (doc.hidden) {
      // 浏览器会自动释放，这里主动归还以同步状态；期间新申请也会被 generation 作废
      void releaseSentinel()
    } else {
      void apply() // 回到前台：如果业务仍需要，重新申请
    }
  }

  function bind() {
    if (listening || !doc || typeof doc.addEventListener !== 'function') return
    doc.addEventListener('visibilitychange', onVisibility)
    listening = true
  }

  function unbind() {
    if (!listening || !doc || typeof doc.removeEventListener !== 'function') return
    doc.removeEventListener('visibilitychange', onVisibility)
    listening = false
  }

  async function dispose() {
    wanted = false
    unbind()
    await releaseSentinel()
  }

  return { supported, locked, setWanted, bind, unbind, dispose, apply }
}

/** 组件内使用的封装：卸载时自动清理锁与事件监听。 */
export function useWakeLock() {
  const wake = createWakeLock()
  onBeforeUnmount(() => { void wake.dispose() })
  return wake
}
