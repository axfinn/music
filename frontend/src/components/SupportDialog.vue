<script setup>
import { computed } from 'vue'
import { siteInfo } from '../site.js'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})

// 站名来自 /api/site，不写死在前端产物里（与顶栏一致）。
const site = siteInfo()
</script>

<template>
  <el-dialog
    v-model="visible"
    class="support-dialog"
    :width="440"
    align-center
    append-to-body
    :close-on-click-modal="true"
  >
    <template #header>
      <div class="sd-head">
        <span class="sd-emoji">☕</span>
        <span class="sd-title">给作者加点电</span>
      </div>
    </template>

    <div class="sd-body">
      <p class="sd-lead">嗨，这里是 <b>{{ site.name }}</b> 的作者，一只用爱发电的野生程序员。</p>

      <p class="sd-text">
        这个站纯粹是我自己捣鼓出来的：<b>没有广告、没有会员、不卖课</b>，
        连曲库里的歌都是一首首亲手存进来的。唯一在悄悄烧钱的，是服务器和带宽——
        你每听一首歌，流量都在替我交电费。
      </p>

      <p class="sd-text">
        如果你已经在这儿听了三首歌，说明它对你还算有点用 🎧。
        要是手头刚好宽裕，<b>请我喝杯咖啡</b>，帮带宽续个命？🥺
      </p>

      <p class="sd-text sd-muted">
        不投也完全 OK，继续白嫖听歌，作者也很开心 :)<br />
        每一分都会被拿去续服务器，绝不变现、绝不打扰。
      </p>

      <div class="sd-qrs">
        <figure class="sd-qr">
          <img src="/support/alipay.jpeg" alt="支付宝收款码" loading="lazy" />
          <figcaption>支付宝</figcaption>
        </figure>
        <figure class="sd-qr">
          <img src="/support/wxpay.jpeg" alt="微信支付收款码" loading="lazy" />
          <figcaption>微信支付</figcaption>
        </figure>
      </div>

      <p class="sd-thanks">—— 比心，谢谢你的每一首歌 🎶</p>
    </div>

    <template #footer>
      <el-button class="sd-close" @click="visible = false">我先听歌去啦 →</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.support-dialog :deep(.el-dialog) {
  background: #14171f;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  max-width: calc(100vw - 32px);
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.55);
}
.support-dialog :deep(.el-dialog__header) {
  padding: 18px 22px 0;
  margin-right: 0;
}
.support-dialog :deep(.el-dialog__title) { color: #e8eaf0; }
.support-dialog :deep(.el-dialog__body) { padding: 10px 22px 2px; }
.support-dialog :deep(.el-dialog__footer) { padding: 10px 22px 20px; }

.sd-head { display: flex; align-items: center; gap: 10px; }
.sd-emoji { font-size: 22px; line-height: 1; }
.sd-title { font-size: 17px; font-weight: 650; color: #e8eaf0; }

.sd-body { font-size: 13.5px; line-height: 1.75; color: #c3c9d6; }
.sd-lead { margin: 2px 0 12px; color: #e8eaf0; }
.sd-text { margin: 0 0 11px; }
.sd-text b { color: #ffb055; font-weight: 600; }
.sd-muted { color: #8a93a5; font-size: 12.5px; }

.sd-qrs {
  display: flex;
  gap: 18px;
  justify-content: center;
  flex-wrap: wrap;
  margin: 16px 0 8px;
}
.sd-qr { margin: 0; text-align: center; }
.sd-qr img {
  width: 150px;
  height: auto;
  background: #fff;
  border-radius: 12px;
  padding: 8px;
  box-shadow: 0 4px 18px rgba(0, 0, 0, 0.3);
}
.sd-qr figcaption { margin-top: 8px; font-size: 12.5px; color: #9aa3b5; }

.sd-thanks { text-align: center; color: #8a93a5; font-size: 12px; margin: 10px 0 0; }

.sd-close {
  width: 100%;
  border: none;
  color: #fff;
  font-weight: 600;
  border-radius: 10px;
  background: linear-gradient(135deg, #f2592a, #ffb055);
}
.sd-close:hover,
.sd-close:focus { color: #fff; filter: brightness(1.06); }
</style>
