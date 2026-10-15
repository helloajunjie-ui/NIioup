<template>
  <div class="gate">
    <div class="topbar">
      <span class="brand">{{ info.site_title || '文件仓库' }}<span class="dot">.</span></span>
      <button class="btn" @click="closeWin">关闭</button>
    </div>

    <div class="card gcard">
      <div v-if="err && !file" class="err center">{{ err }}</div>

      <template v-else>
        <!-- direct 无广直连：不等待、无广告，直接下载 -->
        <div v-if="mode === 'direct'" class="directbox">
          <div class="dl-big">直接下载</div>
          <div class="lbl">该站点已开启无广告直连下载</div>
        </div>

        <!-- ad 广告模式：顶部展示广告位 -->
        <div v-else-if="mode === 'ad' && info.ad_code" class="adslot" v-html="info.ad_code"></div>

        <!-- 倒计时（无广告时展示大倒计时） -->
        <div v-else class="count">
          <div class="num">{{ remain }}</div>
          <div class="lbl">下载将在 {{ remain }} 秒后可用</div>
        </div>

        <!-- 文件信息 -->
        <div class="frow" v-if="file">
          <div class="tile">{{ (file.ext || '?').slice(0, 4) }}</div>
          <div class="finfo">
            <div class="name">{{ file.name }}</div>
            <div class="meta">{{ formatSize(file.size) }} · {{ file.category }}</div>
          </div>
        </div>

        <div class="progress" v-if="mode === 'countdown'"><div :style="{ width: pct + '%' }"></div></div>

        <button class="btn primary dl" :disabled="!ready" @click="download">
          {{ downloading ? '获取中…' : ready ? '开始下载' : '请等待 ' + remain + ' 秒' }}
        </button>
        <div class="note" v-if="mode !== 'direct'">提示：等待倒计时结束后点击「开始下载」即可获取文件，可随时关闭本窗口。</div>
        <div v-if="err" class="err center">{{ err }}</div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { api, formatSize } from '../api.js'

const route = useRoute()
const id = route.params.id

const info = ref({ wait_sec_min: 5, wait_sec_max: 20, ad_code: '', site_title: '', download_mode: 'countdown' })
const file = ref(null)
const err = ref('')
const remain = ref(0)
const total = ref(0)
const downloading = ref(false)
let timer = null

const mode = computed(() => (info.value.download_mode === 'ad' && !info.value.ad_code ? 'countdown' : info.value.download_mode))
// direct / ad：立即可下载；countdown：倒计时结束才可用
const ready = computed(() => mode.value !== 'countdown' || remain.value <= 0)
const pct = computed(() => (total.value ? Math.round(((total.value - remain.value) / total.value) * 100) : 0))

onMounted(async () => {
  try {
    info.value = await api.info()
    file.value = await api.fileInfo(id)
    const min = Math.max(0, Number(info.value.wait_sec_min) || 5)
    const max = Math.max(min, Number(info.value.wait_sec_max) || 20)
    total.value = min + Math.floor(Math.random() * (max - min + 1))
    remain.value = total.value
  } catch (e) {
    err.value = e.message || '加载失败'
  }
  timer = setInterval(() => {
    if (remain.value > 0) remain.value--
  }, 1000)
})

onBeforeUnmount(() => timer && clearInterval(timer))

async function download() {
  if (downloading.value || !ready.value) return
  downloading.value = true
  err.value = ''
  try {
    if (mode.value === 'direct') {
      window.location.href = '/f/' + id + '?dl=1'
      return
    }
    const res = await api.dlStart(id)
    window.location.href = res.url
  } catch (e) {
    err.value = e.message || '获取下载链接失败'
    downloading.value = false
  }
}

function closeWin() {
  if (window.opener && !window.opener.closed) window.close()
  else window.location.href = '/'
}
</script>

<style scoped>
.gate { max-width: 460px; margin: 0 auto; padding: 12px 14px 40px; }
.topbar { display: flex; align-items: center; justify-content: space-between; padding: 10px 0 18px; }
.brand { font-size: 16px; font-weight: 700; }
.brand .dot { color: var(--accent); }
.gcard { text-align: center; }
.adslot {
  margin-bottom: 18px; border: 1px solid var(--border); border-radius: var(--radius);
  overflow: hidden; min-height: 90px; display: flex; align-items: center; justify-content: center;
  background: #fafbfc;
}
.adslot :deep(img) { max-width: 100%; display: block; }
.count { padding: 10px 0 6px; }
.count .num { font-size: 56px; font-weight: 700; color: var(--accent); line-height: 1.1; font-variant-numeric: tabular-nums; }
.count .lbl { color: var(--muted); font-size: 13px; }
.directbox { padding: 18px 0 6px; }
.directbox .dl-big { font-size: 22px; font-weight: 700; color: var(--ok); }
.directbox .lbl { color: var(--muted); font-size: 13px; margin-top: 4px; }
.frow { display: flex; align-items: center; gap: 12px; text-align: left; margin: 20px 0 6px; }
.tile {
  width: 48px; height: 48px; border-radius: 10px; flex: none;
  display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, #697586, #4b5563); color: #fff;
  font-size: 13px; font-weight: 700; text-transform: uppercase;
}
.finfo { min-width: 0; }
.name { font-size: 14px; font-weight: 600; word-break: break-all; }
.meta { font-size: 12px; color: var(--muted); }
.dl { width: 100%; margin-top: 18px; padding: 11px 0; font-size: 15px; }
.note { font-size: 12px; color: var(--muted); margin-top: 10px; }
</style>
