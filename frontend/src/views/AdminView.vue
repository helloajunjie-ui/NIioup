<script setup>
import { ref, onMounted } from 'vue'
import { api, getToken, setToken, clearToken, formatSize, formatTime, origin } from '../api.js'

const authed = ref(!!getToken())
const pwd = ref('')
const loginErr = ref('')
const logging = ref(false)
const tab = ref('files')

async function login() {
  loginErr.value = ''
  logging.value = true
  try {
    const d = await api.login(pwd.value)
    setToken(d.token)
    authed.value = true
    pwd.value = ''
    await loadFiles(); await loadConfig()
  } catch (e) {
    loginErr.value = e.message
  } finally { logging.value = false }
}
function logout() { clearToken(); authed.value = false; tab.value = 'files' }

// ---- 文件管理 ----
const files = ref([])
const total = ref(0)
const q = ref('')
const offset = ref(0)
const pageSize = 100
const listMsg = ref('')
let qTimer = null

async function loadFiles() {
  try {
    const d = await api.adminFiles(q.value, offset.value, pageSize)
    files.value = d.files || []
    total.value = d.total || 0
  } catch (e) { listMsg.value = e.message }
}
function onSearch() { offset.value = 0; clearTimeout(qTimer); qTimer = setTimeout(loadFiles, 300) }
function pageTo(n) { offset.value = n; loadFiles() }

async function del(f) {
  if (!confirm(`删除「${f.name}」？此操作不可恢复。`)) return
  try { await api.adminDelete(f.id); await loadFiles(); listMsg.value = '已删除' } catch (e) { listMsg.value = e.message }
}

// ---- 设置 ----
const cfg = ref({})
const saveMsg = ref('')
const cfgErr = ref('')
async function loadConfig() {
  try { cfg.value = await api.getConfig() } catch (e) { cfgErr.value = e.message }
}
const extArr = (s) => (s || '').split(/[,，\s]+/).filter(Boolean)

async function saveConfig() {
  saveMsg.value = ''; cfgErr.value = ''
  const body = {
    site_title: cfg.value.site_title,
    port: Number(cfg.value.port),
    storage_dir: cfg.value.storage_dir,
    frontend_dir: cfg.value.frontend_dir,
    max_file_size_mb: Number(cfg.value.max_file_size_mb),
    allowed_extensions: extArr(cfg.value.allowed_extensions),
    max_files_total: Number(cfg.value.max_files_total),
    upload_enabled: !!cfg.value.upload_enabled,
    auto_delete_days: Number(cfg.value.auto_delete_days || 0),
    public_list: !!cfg.value.public_list,
    upload_rate_per_min: Number(cfg.value.upload_rate_per_min || 0),
    download_rate_per_min: Number(cfg.value.download_rate_per_min || 0),
    ad_code: cfg.value.ad_code || '',
    wait_sec_min: Number(cfg.value.wait_sec_min || 0),
    wait_sec_max: Number(cfg.value.wait_sec_max || 0),
  }
  if (cfg.value.new_password) body.admin_password = cfg.value.new_password
  try {
    await api.putConfig(body)
    saveMsg.value = '配置已保存'
    cfg.value.new_password = ''
    await loadConfig()
  } catch (e) { cfgErr.value = e.message }
}

// ---- 备份 / 恢复 ----
const backing = ref(false)
const backProgress = ref(0)
const backMsg = ref('')
async function backup() {
  backing.value = true; backProgress.value = 0; backMsg.value = ''
  try {
    const blob = await api.backup((p) => (backProgress.value = p))
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'backup_' + new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '') + '.zip'
    document.body.appendChild(a); a.click(); a.remove()
    URL.revokeObjectURL(url)
    backMsg.value = '备份已生成并开始下载'
  } catch (e) { backMsg.value = e.message }
  finally { backing.value = false }
}

const restFile = ref(null)
const restInput = ref(null)
const restoring = ref(false)
const restProgress = ref(0)
const restMsg = ref('')
function onRestoreFile(e) { restFile.value = e.target.files?.[0] || null }
async function restore() {
  restMsg.value = ''
  if (!restFile.value) { restMsg.value = '请先选择备份 zip 文件'; return }
  if (!confirm('恢复会用备份包覆盖当前全部文件与配置，确定继续？')) return
  restoring.value = true; restProgress.value = 0
  try {
    const d = await api.restore(restFile.value, (p) => (restProgress.value = p))
    restMsg.value = `恢复成功，共 ${d.restored} 个文件。` + (d.note ? ' ' + d.note : '')
    restFile.value = null
  } catch (e) { restMsg.value = e.message }
  finally { restoring.value = false }
}

onMounted(() => { if (authed.value) { loadFiles(); loadConfig() } })
</script>

<template>
  <div>
    <!-- 登录 -->
    <section class="card" v-if="!authed" style="max-width:380px;margin:40px auto">
      <h3>管理员登录</h3>
      <div class="field">
        <label>密码</label>
        <input class="input" type="password" v-model="pwd" placeholder="站长密码"
          @keyup.enter="login" />
        <div class="err" v-if="loginErr">{{ loginErr }}</div>
      </div>
      <button class="btn primary" style="width:100%" :disabled="logging" @click="login">
        {{ logging ? '登录中…' : '登录' }}
      </button>
    </section>

    <!-- 后台 -->
    <div v-else>
      <div class="card" style="display:flex;justify-content:space-between;align-items:center">
        <div class="nav">
          <button class="btn" :class="tab === 'files' && 'primary'" @click="tab='files'">文件管理</button>
          <button class="btn" :class="tab === 'config' && 'primary'" @click="tab='config'">设置</button>
          <button class="btn" :class="tab === 'backup' && 'primary'" @click="tab='backup'">一键备份</button>
        </div>
        <button class="btn danger" @click="logout">退出</button>
      </div>

      <!-- 文件管理 -->
      <section class="card" v-if="tab === 'files'">
        <h3>文件列表 <span class="muted" style="font-weight:400;font-size:13px">共 {{ total }} 个</span></h3>
        <div class="field">
          <input class="input" v-model="q" placeholder="搜索文件名或 ID…" @input="onSearch" />
        </div>
        <p class="empty" v-if="!files.length">暂无文件</p>
        <table class="list" v-else>
          <thead><tr><th>名称</th><th>分类</th><th>游客</th><th>大小</th><th>时间</th><th>下载</th><th>ID</th><th></th></tr></thead>
          <tbody>
            <tr v-for="f in files" :key="f.id">
              <td class="nowrap" style="max-width:200px;overflow:hidden;text-overflow:ellipsis"
                :title="f.name">{{ f.name }}</td>
              <td class="nowrap muted">{{ f.category }}</td>
              <td class="mono nowrap muted">{{ f.guest_uid }}</td>
              <td class="nowrap">{{ formatSize(f.size) }}</td>
              <td class="nowrap muted">{{ formatTime(f.uploaded_at) }}</td>
              <td class="nowrap">{{ f.downloads }}</td>
              <td class="mono nowrap">{{ f.id }}</td>
              <td class="nowrap" style="display:flex;gap:6px">
                <a class="btn" :href="f.url + '?dl=1'" target="_blank" rel="noopener">链接</a>
                <button class="btn danger" @click="del(f)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="pager" v-if="total > pageSize">
          <button class="btn" :disabled="offset === 0" @click="pageTo(0)">首页</button>
          <span>{{ offset + 1 }}-{{ Math.min(offset + pageSize, total) }} / {{ total }}</span>
          <button class="btn" :disabled="offset + pageSize >= total" @click="pageTo(offset + pageSize)">下一页</button>
        </div>
        <div class="dim" style="margin-top:8px" v-if="listMsg">{{ listMsg }}</div>
      </section>

      <!-- 设置 -->
      <section class="card" v-if="tab === 'config'">
        <h3>站点设置</h3>
        <div class="row">
          <div class="field"><label>站点名称</label>
            <input class="input" v-model="cfg.site_title" /></div>
          <div class="field"><label>端口</label>
            <input class="input" type="number" v-model="cfg.port" /></div>
        </div>
        <div class="field"><label>存储目录</label>
          <input class="input" v-model="cfg.storage_dir" />
          <div class="hint">相对 backend 运行目录的路径或绝对路径，重启后生效</div></div>
        <div class="field"><label>前端目录</label>
          <input class="input" v-model="cfg.frontend_dir" /></div>
        <div class="row">
          <div class="field"><label>单文件上限 (MB)</label>
            <input class="input" type="number" v-model="cfg.max_file_size_mb" /></div>
          <div class="field"><label>文件总数上限 (0=不限)</label>
            <input class="input" type="number" v-model="cfg.max_files_total" /></div>
        </div>
        <div class="field"><label>允许的文件类型（逗号分隔，留空=全部允许）</label>
          <input class="input" v-model="cfg.allowed_extensions" placeholder="如 jpg, png, zip, pdf" />
          <div class="hint">示例：jpg, png, zip, pdf</div></div>
        <div class="row">
          <div class="field"><label>上传频率（次/分钟/IP，0=不限）</label>
            <input class="input" type="number" v-model="cfg.upload_rate_per_min" /></div>
          <div class="field"><label>下载频率（次/分钟/IP，0=不限）</label>
            <input class="input" type="number" v-model="cfg.download_rate_per_min" /></div>
        </div>
        <div class="row">
          <div class="field"><label>下载等待最短秒数</label>
            <input class="input" type="number" v-model="cfg.wait_sec_min" /></div>
          <div class="field"><label>下载等待最长秒数</label>
            <input class="input" type="number" v-model="cfg.wait_sec_max" /></div>
        </div>
        <div class="field"><label>下载门槛页广告位（HTML，留空=显示倒计时）</label>
          <textarea class="input" rows="3" v-model="cfg.ad_code" placeholder="粘贴你自己的广告代码，如 广告联盟 iframe/图片 链接"></textarea>
          <div class="hint">填写后，下载门槛页顶部会展示这段广告；留空则显示 5–20 秒随机倒计时。</div></div>
        <div class="field"><label>自动删除（超过 N 天删除，0=永久保留）</label>
          <input class="input" type="number" v-model="cfg.auto_delete_days" /></div>
        <div class="field check" style="margin-bottom:8px">
          <input type="checkbox" v-model="cfg.upload_enabled" /> 开放匿名上传
        </div>
        <div class="field check" style="margin-bottom:16px">
          <input type="checkbox" v-model="cfg.public_list" /> 首页展示最近分享
        </div>
        <div class="field"><label>修改管理员密码（留空则不修改）</label>
          <input class="input" type="password" v-model="cfg.new_password" /></div>
        <div class="err" v-if="cfgErr">{{ cfgErr }}</div>
        <div class="ok" v-if="saveMsg">{{ saveMsg }}</div>
        <button class="btn primary" @click="saveConfig">保存设置</button>
      </section>

      <!-- 备份 / 恢复 -->
      <section class="card" v-if="tab === 'backup'">
        <h3>一键备份</h3>
        <p class="dim">打包所有已上传文件 + 站点配置 + 文件元数据，生成 zip 下载。文件较多时可能较慢。</p>
        <div class="progress" v-if="backing"><div :style="{ width: backProgress + '%' }"></div></div>
        <button class="btn primary" :disabled="backing" @click="backup">
          {{ backing ? '正在生成…' : '开始备份' }}
        </button>
        <div class="ok" v-if="backMsg">{{ backMsg }}</div>

        <div style="border-top:1px dashed var(--border);margin-top:20px;padding-top:18px">
          <h3>一键恢复</h3>
          <p class="dim">上传之前导出的备份 zip，覆盖式恢复全部文件、配置与元数据。</p>
          <input ref="restInput" type="file" accept=".zip" hidden @change="onRestoreFile" />
          <div style="display:flex;gap:8px;align-items:center;margin-top:10px">
            <input class="input" readonly :value="restFile ? restFile.name : '未选择备份文件'" @click="restInput.click()" placeholder="点击选择备份 .zip" />
            <button class="btn primary" :disabled="restoring" @click="restore">
              {{ restoring ? '恢复中…' : '开始恢复' }}
            </button>
          </div>
          <div class="progress" v-if="restoring"><div :style="{ width: restProgress + '%' }"></div></div>
          <div class="ok" v-if="restMsg">{{ restMsg }}</div>
        </div>
      </section>
    </div>
  </div>
</template>
