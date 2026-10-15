<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, ensureGuestUid, formatSize, timeAgo, copyText, origin } from '../api.js'

const MAX_FILES = 5

const info = ref({ upload_enabled: true, allowed_extensions: [], max_file_size_mb: 100, public_list: true })
// 上传
const dragging = ref(false)
const uploading = ref(false)
const total = ref(0)
const done = ref(0)
const curName = ref('')
const curPct = ref(0)
const fileInput = ref(null)
const upErr = ref('')
const category = ref('')
const guestUid = ref('')
const results = ref([])
const copied = ref('')
// 分享列表
const files = ref([])
const categories = ref([])
const activeTab = ref('所有')
const search = ref('')
const totalFiles = ref(0)
const listErr = ref('')
// 删除口令
const delToken = ref('')
const deleting = ref(false)
const delMsg = ref('')

let searchTimer = null

onMounted(async () => {
  guestUid.value = ensureGuestUid()
  try {
    info.value = await api.info()
  } catch (e) { listErr.value = e.message }
  await loadFiles()
})

async function loadFiles() {
  try {
    const d = await api.publicFiles(200, search.value.trim(), activeTab.value)
    files.value = d.files || []
    totalFiles.value = d.total || 0
    categories.value = ['所有', ...(d.categories || []).filter((c) => c !== '未分类' && c !== '')]
    if (!categories.value.includes(activeTab.value)) activeTab.value = '所有'
  } catch (e) { listErr.value = e.message }
}
function onSearch() { clearTimeout(searchTimer); searchTimer = setTimeout(loadFiles, 300) }
function setTab(c) { activeTab.value = c; loadFiles() }

// ---- 上传 ----
function pick() { if (!uploading.value) fileInput.value.click() }
function onDrop(e) {
  dragging.value = false
  const f = Array.from(e.dataTransfer?.files || [])
  if (f.length) uploadBatch(f)
}
function onSelect(e) {
  const f = Array.from(e.target.files || [])
  e.target.value = ''
  if (f.length) uploadBatch(f)
}
async function uploadBatch(fs) {
  upErr.value = ''; results.value = []
  if (fs.length > MAX_FILES) { upErr.value = `一次最多上传 ${MAX_FILES} 个文件，已取前 ${MAX_FILES} 个`; fs = fs.slice(0, MAX_FILES) }
  if (!info.value.upload_enabled) { upErr.value = '上传已关闭'; return }
  total.value = fs.length; done.value = 0; uploading.value = true
  for (const f of fs) {
    curName.value = f.name; curPct.value = 0
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
    results.value.push(ok); done.value++
  }
  uploading.value = false; curName.value = ''
  await loadFiles()
}

function fullUrl(u) { return origin() + u }
async function copy(text, key) {
  try { await copyText(text); copied.value = key; setTimeout(() => (copied.value = ''), 1500) } catch (_) {}
}

// ---- 删除口令 ----
async function removeFile() {
  delMsg.value = ''
  if (!delToken.value.trim()) { delMsg.value = '请先粘贴删除口令'; return }
  if (!confirm('确定删除该文件？此操作不可恢复。')) return
  deleting.value = true
  try {
    await api.selfDelete(delToken.value.trim())
    delMsg.value = '删除成功'; delToken.value = ''
    await loadFiles()
  } catch (e) { delMsg.value = e.message } finally { deleting.value = false }
}

// ---- 卡片图标配色 ----
const palettes = [
  ['#3b7cff', '#2f66d8'], ['#30a46c', '#21835a'], ['#e5484d', '#c2343a'],
  ['#f76b15', '#d9570a'], ['#8e4ec6', '#7238a6'], ['#12a5b4', '#0b8290'],
  ['#5c6ac4', '#3f4c9e'], ['#d6409f', '#b52a82'], ['#697586', '#4b5563'],
]
function tileColor(ext) {
  let h = 0
  for (const ch of (ext || 'file')) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return palettes[h % palettes.length]
}
const shownFiles = computed(() => files.value)
</script>

<template>
  <div>
    <!-- Hero + 搜索 -->
    <section class="hero">
      <h1>匿名分享，无需登录</h1>
      <p>拖拽即可上传，每个文件自动生成独立外链</p>
      <div class="searchbox">
        <input class="input" v-model="search" placeholder="搜索文件…" @input="onSearch" @keyup.enter="loadFiles" />
      </div>
    </section>

    <!-- 上传 -->
    <section class="card">
      <input ref="fileInput" type="file" multiple hidden @change="onSelect" />
      <div class="field">
        <label>分类（选填，用于首页分组）</label>
        <input class="input" v-model="category" placeholder="如：图片、文档、素材…" :disabled="uploading" />
      </div>
      <div class="dim" style="margin:-6px 0 10px">游客编号：<span class="mono">{{ guestUid }}</span>（自动生成，区分访客）</div>

      <div class="drop" :class="{ drag: dragging }"
        @dragover.prevent="dragging = true" @dragleave.prevent="dragging = false"
        @drop.prevent="onDrop" @click="pick">
        <div class="big" v-if="!uploading">把文件拖到这里，或点击选择（最多 {{ MAX_FILES }} 个）</div>
        <div class="big" v-else>上传中 {{ done }}/{{ total }}：{{ curName }} {{ curPct }}%</div>
        <div class="small">每个文件自动生成独立外链</div>
      </div>
      <div class="progress" v-if="uploading"><div :style="{ width: curPct + '%' }"></div></div>

      <div class="tip">
        <span class="tag" v-if="info.max_file_size_mb">单文件 ≤ {{ info.max_file_size_mb }} MB</span>
        <span class="tag" v-if="info.allowed_extensions && info.allowed_extensions.length" v-for="e in info.allowed_extensions" :key="e">. {{ e }}</span>
        <span class="tag" v-else>类型不限</span>
        <span class="tag">一次最多 {{ MAX_FILES }} 个</span>
        <span class="tag" v-if="!info.upload_enabled">上传已关闭</span>
      </div>
      <div class="err" v-if="upErr">{{ upErr }}</div>
    </section>

    <!-- 上传结果 -->
    <section class="card" v-if="results.length">
      <h3>上传结果（{{ results.length }} 个）</h3>
      <template v-for="(r, i) in results" :key="i">
        <div v-if="r.url" style="padding:8px 0;border-top:1px dashed var(--border)">
          <div class="ok" style="margin-bottom:4px">✔ {{ r.name }} · {{ formatSize(r.size) }} · {{ r.category }}</div>
          <div class="linkbox">
            <input class="input mono" readonly :value="fullUrl(r.url)" />
            <button class="btn" @click="copy(fullUrl(r.url), 'link' + i)">{{ copied === 'link' + i ? '已复制' : '复制' }}</button>
            <a class="btn primary" :href="r.url + '?dl=1'" target="_blank" rel="noopener">下载</a>
          </div>
          <div class="dim">删除口令：</div>
          <div class="tokenbox">{{ r.delete_token }}
            <button class="btn" style="margin-left:8px" @click="copy(r.delete_token, 'tok' + i)">{{ copied === 'tok' + i ? '已复制' : '复制' }}</button>
          </div>
        </div>
        <div v-else class="err" style="padding-top:8px">✘ {{ r.name }}：{{ r.error }}</div>
      </template>
    </section>

    <!-- 分享列表 -->
    <section v-if="info.public_list">
      <div class="tabs">
        <button class="tab" :class="{ active: activeTab === c }" v-for="c in categories" :key="c" @click="setTab(c)">{{ c }}</button>
      </div>
      <div class="dim" style="margin-bottom:12px">{{ totalFiles }} 个文件</div>

      <p class="empty" v-if="!files.length">还没有文件，去上传第一个吧</p>
      <p class="err" v-else-if="listErr">{{ listErr }}</p>

      <div class="grid" v-else>
        <div class="filecard" v-for="f in shownFiles" :key="f.id">
          <a class="ftile" :href="f.url" target="_blank" rel="noopener"
            :style="{ background: 'linear-gradient(135deg, ' + tileColor(f.ext)[0] + ', ' + tileColor(f.ext)[1] + ')' }">
            <span>{{ (f.ext || 'file').toUpperCase().slice(0, 4) }}</span>
          </a>
          <div class="fname" :title="f.name">{{ f.name }}</div>
          <div class="fmeta">{{ (f.ext || 'file').toUpperCase() }} · {{ formatSize(f.size) }}
            <span class="fcat" v-if="f.category && f.category !== '未分类'">{{ f.category }}</span>
          </div>
          <div class="fup">游客 <span class="mono">{{ f.guest_uid }}</span> · {{ timeAgo(f.uploaded_at) }}</div>
          <div class="fact">
            <a class="btn primary" style="flex:1" :href="f.url + '?dl=1'" target="_blank" rel="noopener">下载</a>
            <button class="btn" @click="copy(fullUrl(f.url), 'g' + f.id)">{{ copied === 'g' + f.id ? '已复制' : '复制' }}</button>
          </div>
        </div>
      </div>
    </section>

    <!-- 删除口令 -->
    <section class="card">
      <h3>删除文件（凭口令匿名删除）</h3>
      <p class="dim" style="margin-bottom:10px">把上传成功时显示的删除口令粘贴到这里，即可匿名删除对应文件。</p>
      <div class="linkbox">
        <input class="input mono" v-model="delToken" placeholder="粘贴删除口令…" @keyup.enter="removeFile" />
        <button class="btn danger" :disabled="deleting" @click="removeFile">{{ deleting ? '删除中…' : '删除' }}</button>
      </div>
      <div class="ok" v-if="delMsg === '删除成功'">{{ delMsg }}</div>
      <div class="err" v-else-if="delMsg">{{ delMsg }}</div>
    </section>
  </div>
</template>
