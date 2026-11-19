<script setup>
/** 新建 / 重命名歌单。管理员可选「公开」或「仅本机」，访客只能建本机歌单。 */
import { computed, ref, watch } from 'vue'
import { useMusicContext } from '../context.js'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  playlist: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue'])
const { playlists, isAdmin } = useMusicContext()

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})
const name = ref('')
const description = ref('')
const scope = ref('local')
const busy = ref(false)
const editing = computed(() => !!props.playlist)

watch(() => props.modelValue, open => {
  if (!open) return
  name.value = props.playlist?.name || ''
  description.value = props.playlist?.description || ''
  scope.value = props.playlist?.source || (isAdmin.value ? 'shared' : 'local')
})

async function submit() {
  if (!name.value.trim() || busy.value) return
  busy.value = true
  const action = editing.value
    ? () => playlists.rename(props.playlist, { name: name.value, description: description.value })
    : () => playlists.create({ name: name.value, description: description.value, scope: scope.value })
  const ok = await playlists.run(action, editing.value ? '歌单已更新' : '歌单已创建')
  busy.value = false
  if (ok) visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible" :title="editing ? '编辑歌单' : '新建歌单'"
    width="480px" class="ml-dialog" align-center :close-on-click-modal="false"
  >
    <div class="form">
      <label class="field">
        <span>歌单名称</span>
        <el-input v-model="name" maxlength="80" placeholder="例如：深夜循环" @keyup.enter="submit" />
      </label>
      <label class="field">
        <span>简介（可选）</span>
        <el-input v-model="description" type="textarea" :rows="2" maxlength="500" placeholder="给这个歌单写一句话" />
      </label>
      <div v-if="!editing && isAdmin" class="field">
        <span>可见范围</span>
        <el-radio-group v-model="scope" class="scope">
          <el-radio-button value="shared">公开歌单 · 所有人可见</el-radio-button>
          <el-radio-button value="local">仅本机 · 只有我看得到</el-radio-button>
        </el-radio-group>
        <small class="hint">公开歌单保存在服务器，访客也能播放；本机歌单只存在当前浏览器。</small>
      </div>
      <small v-else-if="!editing" class="hint">访客歌单只保存在当前浏览器，换设备不会同步。</small>
    </div>

    <template #footer>
      <button class="ml-btn" @click="visible = false">取消</button>
      <button class="ml-btn primary" :disabled="!name.trim() || busy" @click="submit">
        {{ busy ? '保存中…' : editing ? '保存修改' : '创建歌单' }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form { display: flex; flex-direction: column; gap: 16px; }
.field { display: flex; flex-direction: column; gap: 7px; }
.field > span { font-size: 12px; font-weight: 700; color: var(--ml-text-2); }
.scope { display: flex; flex-wrap: wrap; gap: 6px; }
.hint { font-size: 11px; line-height: 1.5; color: var(--ml-text-3); }
</style>
