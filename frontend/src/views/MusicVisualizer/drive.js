// 音频驱动层 —— 把 AnalyserNode 的原始频谱压成场景好用的几个量。
//
// 输出的量(每帧更新):
//   freq    Uint8Array  频谱(用于每根柱子 / 每个粒子的频段采样)
//   wave    Uint8Array  时域波形
//   levels  bass / low / mid / high  0..1,攻击快、释放慢(视觉才不会抖成噪点)
//   energy  整体能量,0..1
//   beat    节拍脉冲,命中瞬间 1,之后指数衰减到 0
//   live    是否真的拿到了频谱(没有就退化成合成节拍,画面不会死)
//
// 关键取舍:分析器拿不到数据时(浏览器不支持 / 元素没被接管 / 文件静音)
// 不能什么都不动 —— 页面会像卡死。所以这里内置一个 BPM 合成脉冲兜底。

const clamp01 = v => (v < 0 ? 0 : v > 1 ? 1 : v)
const SMOOTH_ATTACK = 20
const SMOOTH_RELEASE = 4.2
const BEAT_COOLDOWN = 0.24

export function createAudioDrive({ getAnalyser, isPlaying = () => false, bpm = 96 }) {
  const drive = {
    freq: new Uint8Array(1024),
    wave: new Uint8Array(2048),
    levels: { bass: 0, low: 0, mid: 0, high: 0 },
    energy: 0,
    beat: 0,
    live: false,
    hasAnalyser: false,
  }

  let bassHistory = []
  let beatCooldown = 0
  let phase = 0
  let smooth = { bass: 0, low: 0, mid: 0, high: 0, energy: 0 }

  function bandAvg(arr, from, to) {
    const lo = Math.max(0, Math.min(arr.length - 1, from | 0))
    const hi = Math.max(lo + 1, Math.min(arr.length, to | 0))
    let sum = 0
    for (let i = lo; i < hi; i++) sum += arr[i]
    return sum / (hi - lo) / 255
  }

  function approach(current, target, dt, rate) {
    const k = 1 - Math.exp(-dt * rate)
    return current + (target - current) * k
  }

  function update(dt) {
    // dt 异常(切后台回来)时夹一下,避免一次跳变把画面甩飞
    const step = Math.min(Math.max(dt, 0.001), 0.1)
    const an = getAnalyser ? getAnalyser() : null
    drive.hasAnalyser = !!an

    let bass = 0, low = 0, mid = 0, high = 0, energy = 0
    let live = false

    if (an) {
      if (drive.freq.length !== an.frequencyBinCount) drive.freq = new Uint8Array(an.frequencyBinCount)
      if (drive.wave.length !== an.fftSize) drive.wave = new Uint8Array(an.fftSize)
      an.getByteFrequencyData(drive.freq)
      an.getByteTimeDomainData(drive.wave)
      const n = drive.freq.length
      const binHz = 22050 / n // 44.1k 采样 / fftSize 2048
      const at = hz => Math.max(1, Math.min(n, Math.round(hz / binHz)))
      bass = bandAvg(drive.freq, 1, at(160))
      low = bandAvg(drive.freq, at(160), at(520))
      mid = bandAvg(drive.freq, at(520), at(2000))
      high = bandAvg(drive.freq, at(2000), at(9000))
      energy = bandAvg(drive.freq, 1, Math.min(n, at(6000)))
      // 全是 0 → 说明图里没有信号(没接管 / 静音),不能当"安静的歌"处理
      live = energy > 0.006
    }

    drive.live = live

    if (!live) {
      // 兜底:播放中用 BPM 合成一拍一拍的呼吸,暂停时给一点极缓的环境起伏
      phase += step * (bpm / 60)
      if (isPlaying()) {
        const p = phase % 1
        const env = Math.pow(1 - p, 2.6)
        const sway = Math.sin(phase * Math.PI * 2) * 0.04
        bass = 0.3 + env * 0.52 + sway
        low = 0.24 + env * 0.34
        mid = 0.18 + env * 0.2
        high = 0.1 + env * 0.14
        energy = 0.24 + env * 0.4
      } else {
        const s = Math.sin(performance.now() * 0.0007)
        bass = 0.1 + s * 0.025
        low = 0.085
        mid = 0.07 + s * 0.012
        high = 0.045
        energy = 0.075 + s * 0.01
      }
    }

    smooth.bass = approach(smooth.bass, bass, step, bass > smooth.bass ? SMOOTH_ATTACK : SMOOTH_RELEASE)
    smooth.low = approach(smooth.low, low, step, low > smooth.low ? SMOOTH_ATTACK : SMOOTH_RELEASE)
    smooth.mid = approach(smooth.mid, mid, step, mid > smooth.mid ? SMOOTH_ATTACK : SMOOTH_RELEASE)
    smooth.high = approach(smooth.high, high, step, high > smooth.high ? SMOOTH_ATTACK : SMOOTH_RELEASE)
    smooth.energy = approach(smooth.energy, energy, step, energy > smooth.energy ? 14 : 3)

    drive.levels.bass = clamp01(smooth.bass)
    drive.levels.low = clamp01(smooth.low)
    drive.levels.mid = clamp01(smooth.mid)
    drive.levels.high = clamp01(smooth.high)
    drive.energy = clamp01(smooth.energy)

    // 节拍:真频谱用瞬态检测(当前低频 vs 近 40 帧均值),合成模式用相位
    beatCooldown = Math.max(0, beatCooldown - step)
    let hit = false
    if (live) {
      bassHistory.push(drive.levels.bass)
      if (bassHistory.length > 40) bassHistory.shift()
      let avg = 0
      for (const v of bassHistory) avg += v
      avg /= bassHistory.length
      if (beatCooldown === 0 && drive.levels.bass > 0.16 &&
          drive.levels.bass > avg * 1.28 && drive.levels.bass - avg > 0.035) {
        hit = true
      }
    } else if (isPlaying()) {
      const p = phase % 1
      if (beatCooldown === 0 && p < step * (bpm / 60) + 0.02) hit = true
    }
    if (hit) {
      drive.beat = 1
      beatCooldown = BEAT_COOLDOWN
    } else {
      drive.beat = Math.max(0, drive.beat - step * 2.8)
    }

    return drive
  }

  // update 直接挂在 drive 上:引擎每帧只认 drive.update(),不用记住第二个引用
  drive.update = update
  return drive
}
