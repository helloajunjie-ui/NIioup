<script setup>
import { ref, onMounted } from 'vue'
import { api, ensureGuestUid, formatSize, formatTime, copyText, origin } from '../api.js'

const MAX_FILES = 5

const info = ref({ upload_enabled: true, allowed_extensions: [], max_file_size_mb: 100, public_list: true })
const dragging = ref(false)
const uploading = ref(false)
const total = ref(0)
const done = ref(0)
const curName = ref('')
const curPct = ref(0)
const fileInput = ref(null)
const error = ref('')
const category = ref('')
const guestUid = ref('')
const results = ref([])
const recent = ref([])
const copied = ref('')

onMounted(async () => {
  guestUid.value = ensureGuestUid()
  try {
    info.value = await api.info()
    if (info.value.public_list) recent.value = (await api.publicFiles(12)).files || []
  } catch (e) { error.value = e.message }
})

function pick() { if (!uploading.value) fileInput.value.click() }

function onDrop(e) {
  dragging.value = false
  const files = Array.from(e.dataTransfer?.files || [])
  if (files.length) uploadBatch(files)
}
function onSelect(e) {
  const files = Array.from(e.target.files || [])
  e.target.value = ''
  if (files.length) uploadBatch(files)
}

async function uploadBatch(files) {
  error.value = ''
  results.value = []
  if (files.length > MAX_FILES) {
    error.value = `一次最多上传 ${MAX_FILES} 个文件，已取前 ${MAX_FILES} 个`
    files = files.slice(0, MAX_FILES)
  }
  if (!info.value.upload_enabled) { error.value = '上传已关闭'; return }
  total.value = files.length
  done.value = 0
  uploading.value = true

  for (const f of files) {
    curName.value = f.name
    curPct.value = 0
    let ok = null
    if (info.value.max_file_size_mb && f.size > info.value.max_file_size_mb * 1024 * 1024) {
      ok = { name: f.name, error: `超过 ${info.value.max_file_size_mb} MB 限制` }
    } else {
      const exts = info.value.allowed_extensions || []
      if (exts.length) {
        const ext = f.name.split('.').pop()?.toLowerCase() || ''
        if (!exts.includes(ext)) ok = { name: f.name, error: '不允许的类型 .' + ext }
      }
    }
    if (!ok) {
      try {
        const r = await api.upload(f, { guestUid: guestUid.value, category: category.value }, (p) => (curPct.value = p))
        ok = { name: r.name, size: r.size, url: r.url, delete_token: r.delete_token, category: r.category }
      } catch (e) { ok = { name: f.name, error: e.message } }
    }
    results.value.push(ok)
    done.value++
  }
  uploading.value = false
  curName.value = ''
  if (info.value.public_list) recent.value = (await api.publicFiles(12)).files || []
}

function fullUrl(u) { return origin() + u }
async function copy(text, key) {
  try { await copyText(text); copied.value = key; setTimeout(() => (copied.value = ''), 1500) } catch (_) {}
}
</script>

<template>
  <div>
    <section class="card">
      <input ref="fileInput" type="file" multiple hidden @change="onSelect" />

      <div class="field">
        <label>分类（选填，空则记为「未分类」）</label>
        <input class="input" v-model="category" placeholder="如：图片、文档、素材…" :disabled="uploading" />
      </div>
      <div class="dim" style="margin:-6px 0 10px">游客编号：<span class="mono">{{ guestUid }}</span>（自动生成，用于区分访客、防止冲突）</div>

      <div class="drop" :class="{ drag: dragging }"
        @dragover.prevent="dragging = true" @dragleave.prevent="dragging = false"
        @drop.prevent="onDrop" @click="pick">
        <div class="big" v-if="!uploading">把文件拖到这里，或点击选择（可一次选最多 {{ MAX_FILES }} 个）</div>
        <div class="big" v-else>正在上传 {{ done }}/{{ total }}：{{ curName }} {{ curPct }}%</div>
        <div class="small">无需登录，每个文件自动生成独立外链</div>
      </div>
      <div class="progress" v-if="uploading"><div :style="{ width: curPct + '%' }"></div></div>

      <div class="tip">
        <span class="tag" v-if="info.max_file_size_mb">单文件 ≤ {{ info.max_file_size_mb }} MB</span>
        <template v-if="info.allowed_extensions && info.allowed_extensions.length">
          <span class="tag" v-for="e in info.allowed_extensions" :key="e">. {{ e }}</span>
        </template>
        <span class="tag" v-else>类型不限</span>
        <span class="tag">一次最多 {{ MAX_FILES }} 个</span>
        <span class="tag" v-if="!info.upload_enabled">上传已关闭</span>
      </div>
      <div class="err" v-if="error">{{ error }}</div>
    </section>

    <section class="card" v-if="results.length">
      <h3>上传结果（{{ results.length }} 个）</h3>
      <template v-for="(r, i) in results" :key="i">
        <div v-if="r.url" class="result" style="border-top:0;margin-top:0;padding-top:0">
          <div class="ok" style="margin-top:6px">✔ {{ r.name }} · {{ formatSize(r.size) }} · {{ r.category }}</div>
          <div class="linkbox">
            <input class="input mono" readonly :value="fullUrl(r.url)" />
            <button class="btn" @click="copy(fullUrl(r.url), 'link' + i)">{{ copied === 'link' + i ? '已复制' : '复制' }}</button>
            <a class="btn primary" :href="r.url + '?dl=1'" target="_blank" rel="noopener">下载</a>
          </div>
          <div class="dim">删除口令（凭此口令可匿名删除，请保存）：</div>
          <div class="tokenbox">{{ r.delete_token }}
            <button class="btn" style="margin-left:8px" @click="copy(r.delete_token, 'tok' + i)">{{ copied === 'tok' + i ? '已复制' : '复制' }}</button>
          </div>
        </div>
        <div v-else class="err" style="margin-top:8px">✘ {{ r.name }}：{{ r.error }}</div>
      </template>
    </section>

    <section class="card" v-if="info.public_list">
      <h3>最近分享</h3>
      <p class="empty" v-if="!recent.length">还没有文件</p>
      <table class="list" v-else>
        <thead><tr><th>名称</th><th>分类</th><th>游客</th><th>大小</th><th>时间</th><th>下载</th><th></th></tr></thead>
        <tbody>
          <tr v-for="f in recent" :key="f.id">
            <td class="nowrap" style="max-width:180px;overflow:hidden;text-overflow:ellipsis">{{ f.name }}</td>
            <td class="nowrap muted">{{ f.category }}</td>
            <td class="mono nowrap muted">{{ f.guest_uid }}</td>
            <td class="nowrap">{{ formatSize(f.size) }}</td>
            <td class="nowrap muted">{{ formatTime(f.uploaded_at) }}</td>
            <td class="nowrap">{{ f.downloads }}</td>
            <td class="nowrap"><a :href="f.url + '?dl=1'" target="_blank" rel="noopener">下载</a></td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>
