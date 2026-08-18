<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  Activity,
  Bot,
  CheckCircle2,
  Download,
  Eye,
  EyeOff,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  Megaphone,
  MessageSquareReply,
  Pause,
  Play,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Smartphone,
  Trash2,
  Users,
} from '@lucide/vue'
import {
  api,
  type AuthStatus,
  type Campaign,
  type Capabilities,
  type DiscoveryTask,
  type Overview,
  type RelaySettings,
  type ReleaseInfo,
  type SystemInfo,
  type TelegramAccount,
} from './api'

type Section = 'overview' | 'accounts' | 'discover' | 'campaigns' | 'relay' | 'settings'

const sections = [
  { id: 'overview' as const, label: '总览', icon: LayoutDashboard, kicker: '运行中心', title: '系统总览' },
  { id: 'accounts' as const, label: '账号矩阵', icon: Users, kicker: '连接管理', title: '账号矩阵' },
  { id: 'discover' as const, label: '数据采集', icon: Search, kicker: '公开数据', title: '数据采集' },
  { id: 'campaigns' as const, label: '营销任务', icon: Megaphone, kicker: '批量执行', title: '营销任务' },
  { id: 'relay' as const, label: '主号中转', icon: MessageSquareReply, kicker: '消息路由', title: '主号中转' },
  { id: 'settings' as const, label: '系统设置', icon: Settings, kicker: '系统管理', title: '系统设置' },
]

const auth = ref<AuthStatus | null>(null)
const authLoading = ref(true)
const authSubmitting = ref(false)
const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const authError = ref('')
const info = ref<SystemInfo | null>(null)
const overview = ref<Overview | null>(null)
const capabilities = ref<Capabilities | null>(null)
const accounts = ref<TelegramAccount[]>([])
const discoveryTasks = ref<DiscoveryTask[]>([])
const campaigns = ref<Campaign[]>([])
const relay = ref<RelaySettings>({ botUsername: '', masterUsername: '', enabled: false })
const release = ref<ReleaseInfo | null>(null)
const loading = ref(true)
const checking = ref(false)
const updating = ref(false)
const saving = ref('')
const updateMessage = ref('')
const error = ref('')
const success = ref('')
const activeSection = ref<Section>('overview')
const accountForm = ref({ phone: '', displayName: '' })
const authorizingAccountID = ref<number | null>(null)
const telegramCode = ref('')
const telegramPassword = ref('')
const discoveryForm = ref({ query: '', sourceType: 'public_chat' })
const campaignForm = ref({ name: '', kind: 'direct_message', target: '', message: '', runAt: '' })

const versionText = computed(() => info.value?.version || 'dev')
const page = computed(() => sections.find((item) => item.id === activeSection.value) || sections[0])
const connected = computed(() => capabilities.value?.connectedAccounts || 0)

function syncSection() {
  const candidate = window.location.hash.replace('#', '') as Section
  const next = sections.some((item) => item.id === candidate) ? candidate : 'overview'
  if (next !== activeSection.value) clearNotices()
  activeSection.value = next
}

function clearNotices() {
  error.value = ''
  success.value = ''
}

function messageFrom(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

async function loadAll() {
  loading.value = true
  clearNotices()
  try {
    const [systemInfo, overviewData, capabilityData, accountData, discoveryData, campaignData, relayData] = await Promise.all([
      api.systemInfo(), api.overview(), api.capabilities(), api.accounts(), api.discovery(), api.campaigns(), api.relay(),
    ])
    info.value = systemInfo
    overview.value = overviewData
    capabilities.value = capabilityData
    accounts.value = accountData
    discoveryTasks.value = discoveryData
    campaigns.value = campaignData
    relay.value = relayData
  } catch (err) {
    error.value = messageFrom(err, '加载失败')
  } finally {
    loading.value = false
  }
}

async function bootstrap() {
  authLoading.value = true
  try {
    auth.value = await api.authStatus()
    if (auth.value.authenticated) await loadAll()
  } catch (err) {
    authError.value = messageFrom(err, '无法连接服务')
  } finally {
    authLoading.value = false
  }
}

async function submitAuth() {
  authError.value = ''
  if (!auth.value?.configured && password.value !== confirmPassword.value) {
    authError.value = '两次输入的密码不一致'
    return
  }
  authSubmitting.value = true
  try {
    if (auth.value?.configured) await api.login(password.value)
    else await api.setup(password.value)
    auth.value = { configured: true, authenticated: true }
    password.value = ''
    confirmPassword.value = ''
    await loadAll()
  } catch (err) {
    authError.value = messageFrom(err, '操作失败')
  } finally {
    authSubmitting.value = false
  }
}

async function logout() {
  try {
    await api.logout()
  } finally {
    auth.value = { configured: true, authenticated: false }
    info.value = null
    overview.value = null
  }
}

async function createAccount() {
  saving.value = 'account'
  clearNotices()
  try {
    await api.createAccount(accountForm.value)
    accountForm.value = { phone: '', displayName: '' }
    accounts.value = await api.accounts()
    overview.value = await api.overview()
    success.value = '账号已登记，等待 Telegram 授权连接。'
  } catch (err) {
    error.value = messageFrom(err, '添加账号失败')
  } finally {
    saving.value = ''
  }
}

async function deleteAccount(id: number) {
  if (!window.confirm('确定删除这个账号记录？')) return
  saving.value = `account-${id}`
  clearNotices()
  try {
    await api.deleteAccount(id)
    accounts.value = await api.accounts()
    overview.value = await api.overview()
    success.value = '账号记录已删除。'
  } catch (err) {
    error.value = messageFrom(err, '删除账号失败')
  } finally {
    saving.value = ''
  }
}

async function requestTelegramCode(item: TelegramAccount) {
  saving.value = `auth-${item.id}`
  clearNotices()
  try {
    const result = await api.requestAccountCode(item.id)
    accounts.value = await api.accounts()
    if (result.status === 'authorized') {
      authorizingAccountID.value = null
      success.value = 'Telegram 账号已授权。'
    } else {
      authorizingAccountID.value = item.id
      telegramCode.value = ''
      success.value = '验证码已发送，请查看 Telegram 或短信。'
    }
  } catch (err) {
    accounts.value = await api.accounts().catch(() => accounts.value)
    error.value = messageFrom(err, '发送验证码失败')
  } finally {
    saving.value = ''
  }
}

function openAccountAuthorization(item: TelegramAccount) {
  authorizingAccountID.value = item.id
  telegramCode.value = ''
  telegramPassword.value = ''
  clearNotices()
}

async function verifyTelegramCode(item: TelegramAccount) {
  saving.value = `auth-${item.id}`
  clearNotices()
  try {
    const result = await api.verifyAccountCode(item.id, telegramCode.value)
    accounts.value = await api.accounts()
    telegramCode.value = ''
    if (result.status === 'password_required') {
      success.value = '验证码正确，请输入 Telegram 两步验证密码。'
    } else {
      authorizingAccountID.value = null
      success.value = 'Telegram 账号已授权。'
    }
  } catch (err) {
    accounts.value = await api.accounts().catch(() => accounts.value)
    error.value = messageFrom(err, '验证验证码失败')
  } finally {
    saving.value = ''
  }
}

async function verifyTelegramPassword(item: TelegramAccount) {
  saving.value = `auth-${item.id}`
  clearNotices()
  try {
    await api.verifyAccountPassword(item.id, telegramPassword.value)
    accounts.value = await api.accounts()
    telegramPassword.value = ''
    authorizingAccountID.value = null
    success.value = 'Telegram 账号已授权。'
  } catch (err) {
    accounts.value = await api.accounts().catch(() => accounts.value)
    error.value = messageFrom(err, '两步验证失败')
  } finally {
    saving.value = ''
  }
}

async function createDiscovery() {
  saving.value = 'discovery'
  clearNotices()
  try {
    await api.createDiscovery(discoveryForm.value)
    discoveryForm.value = { query: '', sourceType: 'public_chat' }
    discoveryTasks.value = await api.discovery()
    success.value = '采集任务已保存，账号连接后可进入执行队列。'
  } catch (err) {
    error.value = messageFrom(err, '创建采集任务失败')
  } finally {
    saving.value = ''
  }
}

async function deleteDiscovery(id: number) {
  saving.value = `discovery-${id}`
  clearNotices()
  try {
    await api.deleteDiscovery(id)
    discoveryTasks.value = await api.discovery()
    success.value = '采集任务已删除。'
  } catch (err) {
    error.value = messageFrom(err, '删除采集任务失败')
  } finally {
    saving.value = ''
  }
}

async function createCampaign() {
  saving.value = 'campaign'
  clearNotices()
  try {
    await api.createCampaign(campaignForm.value)
    campaignForm.value = { name: '', kind: 'direct_message', target: '', message: '', runAt: '' }
    campaigns.value = await api.campaigns()
    overview.value = await api.overview()
    success.value = '营销任务草稿已保存。'
  } catch (err) {
    error.value = messageFrom(err, '创建营销任务失败')
  } finally {
    saving.value = ''
  }
}

async function toggleCampaign(item: Campaign) {
  const next = item.status === 'paused' || item.status === 'draft' ? 'pending_connection' : 'paused'
  saving.value = `campaign-${item.id}`
  clearNotices()
  try {
    await api.updateCampaignStatus(item.id, next)
    campaigns.value = await api.campaigns()
    success.value = next === 'paused' ? '任务已暂停。' : '任务已进入等待账号连接状态。'
  } catch (err) {
    error.value = messageFrom(err, '更新营销任务失败')
  } finally {
    saving.value = ''
  }
}

async function deleteCampaign(id: number) {
  saving.value = `campaign-delete-${id}`
  clearNotices()
  try {
    await api.deleteCampaign(id)
    campaigns.value = await api.campaigns()
    success.value = '营销任务已删除。'
  } catch (err) {
    error.value = messageFrom(err, '删除营销任务失败')
  } finally {
    saving.value = ''
  }
}

async function saveRelay() {
  saving.value = 'relay'
  clearNotices()
  try {
    await api.updateRelay(relay.value)
    relay.value = await api.relay()
    success.value = '中转配置已保存。'
  } catch (err) {
    error.value = messageFrom(err, '保存中转配置失败')
  } finally {
    saving.value = ''
  }
}

async function checkUpdate() {
  checking.value = true
  clearNotices()
  try {
    release.value = await api.checkUpdate()
  } catch (err) {
    error.value = messageFrom(err, '检查更新失败')
  } finally {
    checking.value = false
  }
}

async function applyUpdate() {
  updating.value = true
  clearNotices()
  updateMessage.value = ''
  try {
    const result = await api.applyUpdate()
    release.value = result.release
    updateMessage.value = '新版本已安装，服务正在重启…'
    window.setTimeout(() => window.location.reload(), 4000)
  } catch (err) {
    error.value = messageFrom(err, '升级失败')
  } finally {
    updating.value = false
  }
}

function formatDate(value?: string) {
  if (!value) return '未执行'
  const date = new Date(value.endsWith('Z') ? value : `${value.replace(' ', 'T')}Z`)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

function statusText(value: string) {
  return ({ pending: '待授权', code_sent: '等待验证码', password_required: '等待 2FA', authorized: '已授权', online: '在线', offline: '离线', error: '授权失败', draft: '草稿', paused: '已暂停', pending_connection: '等待账号连接', ready: '就绪', running: '执行中', completed: '已完成', failed: '失败' } as Record<string, string>)[value] || value
}

function sourceText(value: string) {
  return ({ public_chat: '公开群组', public_channel: '公开频道', message_history: '历史消息' } as Record<string, string>)[value] || value
}

function campaignText(value: string) {
  return ({ direct_message: '私聊消息', group_message: '群组消息', join_group: '加入群组' } as Record<string, string>)[value] || value
}

onMounted(() => {
  syncSection()
  window.addEventListener('hashchange', syncSection)
  void bootstrap()
})

onBeforeUnmount(() => window.removeEventListener('hashchange', syncSection))
</script>

<template>
  <div v-if="authLoading" class="auth-loading" aria-label="正在加载">
    <Bot :size="30" /><RefreshCw :size="20" class="spinning" />
  </div>

  <main v-else-if="!auth?.authenticated" class="auth-page">
    <section class="auth-copy">
      <div class="auth-brand"><Bot :size="28" /><span>Teleflow</span></div>
      <div><p class="auth-kicker">私域运营控制台</p><h1>{{ auth?.configured ? '欢迎回来' : '初始化管理员' }}</h1><p>{{ auth?.configured ? '登录后管理账号矩阵、营销任务与消息中转。' : '设置管理员密码，保护此实例中的账号和运营数据。' }}</p></div>
      <small>单一所有者模式 · 本地数据存储</small>
    </section>
    <section class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="submitAuth">
        <div class="form-heading"><LockKeyhole :size="21" /><div><h2>{{ auth?.configured ? '管理员登录' : '创建管理员密码' }}</h2><p>{{ auth?.configured ? '请输入当前实例的管理员密码' : '密码至少需要 8 个字符' }}</p></div></div>
        <label for="password">管理员密码</label>
        <div class="password-field">
          <input id="password" v-model="password" :type="showPassword ? 'text' : 'password'" :autocomplete="auth?.configured ? 'current-password' : 'new-password'" minlength="8" required autofocus />
          <button type="button" :title="showPassword ? '隐藏密码' : '显示密码'" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword"><EyeOff v-if="showPassword" :size="18" /><Eye v-else :size="18" /></button>
        </div>
        <template v-if="!auth?.configured"><label for="confirm-password">确认密码</label><input id="confirm-password" v-model="confirmPassword" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" minlength="8" required /></template>
        <p v-if="authError" class="auth-error">{{ authError }}</p>
        <button class="primary auth-submit" :disabled="authSubmitting" type="submit"><RefreshCw v-if="authSubmitting" :size="17" class="spinning" /><LockKeyhole v-else :size="17" />{{ authSubmitting ? '请稍候' : auth?.configured ? '登录' : '完成初始化' }}</button>
      </form>
    </section>
  </main>

  <div v-else class="shell">
    <aside class="sidebar">
      <div class="brand"><Bot :size="24" /><span>Teleflow</span></div>
      <nav aria-label="主菜单">
        <a v-for="item in sections" :key="item.id" :class="{ active: activeSection === item.id }" :href="`#${item.id}`" :title="item.label" :aria-label="item.label"><component :is="item.icon" :size="18" /><span>{{ item.label }}</span></a>
      </nav>
      <div class="sidebar-foot"><div><span class="status-dot"></span>服务运行中</div><button title="退出登录" aria-label="退出登录" @click="logout"><LogOut :size="16" /></button></div>
    </aside>

    <main class="app-main">
      <header><div><p>{{ page.kicker }}</p><h1>{{ page.title }}</h1></div><button class="icon-button" title="刷新数据" aria-label="刷新数据" @click="loadAll"><RefreshCw :size="18" :class="{ spinning: loading }" /></button></header>
      <p v-if="error" class="notice error">{{ error }}</p>
      <p v-if="success" class="notice success">{{ success }}</p>

      <template v-if="activeSection === 'overview'">
        <section class="metrics" aria-label="核心指标">
          <article><Users :size="20" /><div><span>已托管账号</span><strong>{{ overview?.accounts ?? '-' }}</strong></div></article>
          <article><Activity :size="20" /><div><span>在线账号</span><strong>{{ overview?.online ?? '-' }}</strong></div></article>
          <article><MessageSquareReply :size="20" /><div><span>今日中转</span><strong>{{ overview?.relayedToday ?? '-' }}</strong></div></article>
          <article><Megaphone :size="20" /><div><span>待执行任务</span><strong>{{ overview?.pendingJobs ?? '-' }}</strong></div></article>
        </section>
        <section class="content-grid">
          <div class="panel"><div class="panel-title"><div><h2>服务状态</h2><p>当前实例的基础运行状态</p></div><CheckCircle2 :size="20" /></div><dl><div><dt>应用版本</dt><dd>{{ versionText }}</dd></div><div><dt>构建提交</dt><dd class="hash">{{ info?.commit || 'none' }}</dd></div><div><dt>数据存储</dt><dd>SQLite WAL</dd></div><div><dt>部署模式</dt><dd>单机单体</dd></div></dl></div>
          <div class="panel"><div class="panel-title"><div><h2>连接能力</h2><p>Telegram 与中转服务配置</p></div><Radio :size="20" /></div><dl><div><dt>Telegram API</dt><dd><span class="status" :class="capabilities?.telegramConfigured ? 'ok' : 'muted'">{{ capabilities?.telegramConfigured ? '已配置' : '未配置' }}</span></dd></div><div><dt>中转 Bot Token</dt><dd><span class="status" :class="capabilities?.relayBotConfigured ? 'ok' : 'muted'">{{ capabilities?.relayBotConfigured ? '已配置' : '未配置' }}</span></dd></div><div><dt>在线矩阵账号</dt><dd>{{ connected }}</dd></div><div><dt>已登记账号</dt><dd>{{ accounts.length }}</dd></div></dl></div>
        </section>
      </template>

      <section v-else-if="activeSection === 'accounts'" class="workspace">
        <div class="capability-banner"><Smartphone :size="19" /><div><strong>{{ capabilities?.telegramConfigured ? 'Telegram API 已配置' : '等待配置 Telegram API' }}</strong><p>账号记录会持久保存；真实登录仍需要 API ID/hash、短信验证码和可选 2FA。</p></div></div>
        <form class="workspace-panel form-grid account-form" @submit.prevent="createAccount"><label>手机号<input v-model="accountForm.phone" placeholder="+8613800138000" autocomplete="tel" required /></label><label>账号名称<input v-model="accountForm.displayName" placeholder="例如：获客账号 A" maxlength="80" /></label><button class="primary" :disabled="saving === 'account'"><Plus :size="17" />{{ saving === 'account' ? '添加中' : '添加账号' }}</button></form>
        <div class="workspace-panel"><div class="section-heading"><div><h2>账号列表</h2><p>{{ accounts.length }} 个账号记录</p></div></div><div v-if="accounts.length" class="table-wrap"><table><thead><tr><th>账号</th><th>手机号</th><th>状态</th><th>最后授权</th><th class="account-actions">操作</th></tr></thead><tbody><template v-for="item in accounts" :key="item.id"><tr><td><strong>{{ item.displayName || '未命名账号' }}</strong><small v-if="item.username" class="account-username">@{{ item.username }}</small><small v-if="item.lastError" class="account-error">{{ item.lastError }}</small></td><td>{{ item.phone }}</td><td><span class="status" :class="item.status === 'online' || item.status === 'authorized' ? 'ok' : 'muted'">{{ statusText(item.status) }}</span></td><td>{{ formatDate(item.lastSeenAt) }}</td><td class="account-actions"><button v-if="item.status === 'pending' || item.status === 'offline' || item.status === 'error'" class="secondary compact" :disabled="saving === `auth-${item.id}` || !capabilities?.telegramConfigured" :title="capabilities?.telegramConfigured ? '发送 Telegram 验证码' : '请先配置 Telegram API'" @click="requestTelegramCode(item)"><Smartphone :size="15" />发送验证码</button><button v-else-if="item.status === 'code_sent'" class="secondary compact" @click="openAccountAuthorization(item)">输入验证码</button><button v-else-if="item.status === 'password_required'" class="secondary compact" @click="openAccountAuthorization(item)">输入 2FA</button><button class="icon-button danger" title="删除账号" :disabled="saving === `account-${item.id}`" @click="deleteAccount(item.id)"><Trash2 :size="16" /></button></td></tr><tr v-if="authorizingAccountID === item.id && (item.status === 'code_sent' || item.status === 'password_required')" class="auth-step-row"><td colspan="5"><form v-if="item.status === 'code_sent'" class="account-auth-step" @submit.prevent="verifyTelegramCode(item)"><div><strong>输入 Telegram 验证码</strong><p>验证码发送于 {{ formatDate(item.codeSentAt) }}</p></div><input v-model="telegramCode" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{3,8}" placeholder="验证码" required /><button class="primary" :disabled="saving === `auth-${item.id}`"><ShieldCheck :size="16" />验证</button></form><form v-else class="account-auth-step" @submit.prevent="verifyTelegramPassword(item)"><div><strong>输入两步验证密码</strong><p>密码只用于本次 Telegram SRP 验证，不会保存。</p></div><input v-model="telegramPassword" type="password" autocomplete="current-password" placeholder="2FA 密码" maxlength="256" required /><button class="primary" :disabled="saving === `auth-${item.id}`"><ShieldCheck :size="16" />完成授权</button></form></td></tr></template></tbody></table></div><div v-else class="empty"><Users :size="28" /><p>还没有登记矩阵账号</p></div></div>
      </section>

      <section v-else-if="activeSection === 'discover'" class="workspace">
        <div class="capability-banner"><Search :size="19" /><div><strong>公开群组与频道采集</strong><p>任务条件会持久保存；只有已授权账号才能读取其有权访问的公开数据。</p></div></div>
        <form class="workspace-panel form-grid discovery-form" @submit.prevent="createDiscovery"><label>关键词或用户名<input v-model="discoveryForm.query" placeholder="行业关键词或 @username" minlength="2" maxlength="120" required /></label><label>采集范围<select v-model="discoveryForm.sourceType"><option value="public_chat">公开群组</option><option value="public_channel">公开频道</option><option value="message_history">历史消息</option></select></label><button class="primary" :disabled="saving === 'discovery'"><Plus :size="17" />创建任务</button></form>
        <div class="workspace-panel"><div class="section-heading"><div><h2>采集任务</h2><p>按创建时间倒序排列</p></div></div><div v-if="discoveryTasks.length" class="table-wrap"><table><thead><tr><th>关键词</th><th>范围</th><th>状态</th><th>结果</th><th>创建时间</th><th class="actions">操作</th></tr></thead><tbody><tr v-for="item in discoveryTasks" :key="item.id"><td>{{ item.query }}</td><td>{{ sourceText(item.sourceType) }}</td><td><span class="status muted">{{ statusText(item.status) }}</span></td><td>{{ item.resultCount }}</td><td>{{ formatDate(item.createdAt) }}</td><td class="actions"><button class="icon-button danger" title="删除采集任务" :disabled="saving === `discovery-${item.id}`" @click="deleteDiscovery(item.id)"><Trash2 :size="16" /></button></td></tr></tbody></table></div><div v-else class="empty"><Search :size="28" /><p>还没有采集任务</p></div></div>
      </section>

      <section v-else-if="activeSection === 'campaigns'" class="workspace">
        <div class="capability-banner"><Megaphone :size="19" /><div><strong>任务执行受账号连接状态约束</strong><p>创建、暂停和恢复状态会立即持久化；未连接账号时不会发送任何消息。</p></div></div>
        <form class="workspace-panel form-grid campaign-form" @submit.prevent="createCampaign"><label>任务名称<input v-model="campaignForm.name" placeholder="例如：Web3 首轮触达" maxlength="100" required /></label><label>任务类型<select v-model="campaignForm.kind"><option value="direct_message">私聊消息</option><option value="group_message">群组消息</option><option value="join_group">加入群组</option></select></label><label>目标<input v-model="campaignForm.target" placeholder="受众名称、群组或 t.me 链接" /></label><label>计划时间<input v-model="campaignForm.runAt" type="datetime-local" /></label><label class="full">消息内容<textarea v-model="campaignForm.message" :placeholder="campaignForm.kind === 'join_group' ? '加入群组任务可不填写消息' : '输入发送内容'" maxlength="4096" :required="campaignForm.kind !== 'join_group'"></textarea></label><button class="primary" :disabled="saving === 'campaign'"><Plus :size="17" />保存草稿</button></form>
        <div class="workspace-panel"><div class="section-heading"><div><h2>任务列表</h2><p>{{ campaigns.length }} 个营销任务</p></div></div><div v-if="campaigns.length" class="campaign-list"><article v-for="item in campaigns" :key="item.id"><div class="campaign-main"><div><h3>{{ item.name }}</h3><p>{{ campaignText(item.kind) }} · {{ item.target || '未指定目标' }}</p></div><span class="status muted">{{ statusText(item.status) }}</span></div><p class="campaign-message">{{ item.message }}</p><div class="campaign-meta"><span>计划：{{ formatDate(item.runAt) }}</span><span>成功 {{ item.sentCount }} / 失败 {{ item.failedCount }}</span><div class="row-actions"><button class="secondary" :disabled="saving === `campaign-${item.id}`" @click="toggleCampaign(item)"><Play v-if="item.status === 'draft' || item.status === 'paused'" :size="15" /><Pause v-else :size="15" />{{ item.status === 'draft' || item.status === 'paused' ? '准备执行' : '暂停' }}</button><button class="icon-button danger" title="删除任务" :disabled="saving === `campaign-delete-${item.id}`" @click="deleteCampaign(item.id)"><Trash2 :size="16" /></button></div></div></article></div><div v-else class="empty"><Megaphone :size="28" /><p>还没有营销任务</p></div></div>
      </section>

      <section v-else-if="activeSection === 'relay'" class="workspace relay-layout">
        <div class="capability-banner"><MessageSquareReply :size="19" /><div><strong>{{ capabilities?.relayBotConfigured ? '中转 Bot Token 已配置' : '等待配置中转 Bot Token' }}</strong><p>客户消息由矩阵账号接收，经 Bot 通知主账号；回复必须关联原通知后再由原账号发回。</p></div></div>
        <form class="workspace-panel relay-form" @submit.prevent="saveRelay"><div class="section-heading"><div><h2>中转配置</h2><p>用户名不需要输入 @</p></div><span class="status" :class="relay.enabled ? 'ok' : 'muted'">{{ relay.enabled ? '已启用' : '未启用' }}</span></div><div class="form-grid"><label>Bot 用户名<input v-model="relay.botUsername" placeholder="teleflow_relay_bot" /></label><label>主账号用户名<input v-model="relay.masterUsername" placeholder="your_username" /></label><label class="toggle full"><input v-model="relay.enabled" type="checkbox" /><span>启用主账号消息中转</span></label><button class="primary" :disabled="saving === 'relay'"><ShieldCheck :size="17" />{{ saving === 'relay' ? '保存中' : '保存配置' }}</button></div></form>
        <div class="workspace-panel relay-flow"><div class="section-heading"><div><h2>消息链路</h2><p>主账号无需托管到系统</p></div></div><ol><li><span>1</span><div><strong>客户发送消息</strong><p>消息进入对应矩阵账号。</p></div></li><li><span>2</span><div><strong>Bot 通知主账号</strong><p>通知包含来源账号和原会话关联。</p></div></li><li><span>3</span><div><strong>主账号回复通知</strong><p>系统使用原矩阵账号回传给客户。</p></div></li></ol></div>
      </section>

      <section v-else class="content-grid settings-grid">
        <div class="panel"><div class="panel-title"><div><h2>服务状态</h2><p>当前实例的基础运行状态</p></div><CheckCircle2 :size="20" /></div><dl><div><dt>应用版本</dt><dd>{{ versionText }}</dd></div><div><dt>构建提交</dt><dd class="hash">{{ info?.commit || 'none' }}</dd></div><div><dt>公开地址</dt><dd class="hash">{{ info?.publicUrl || '-' }}</dd></div><div><dt>数据库</dt><dd>SQLite WAL</dd></div></dl></div>
        <div class="panel update-panel"><div class="panel-title"><div><h2>版本升级</h2><p>从 GitHub Releases 获取稳定版本</p></div><ShieldCheck :size="20" /></div><div v-if="!release" class="update-empty"><Download :size="28" /><p>检查是否有可用的新版本</p></div><div v-else class="release-result"><strong v-if="!release.configured">尚未配置 GitHub 仓库</strong><strong v-else-if="release.available">发现新版本 {{ release.latestVersion }}</strong><strong v-else>当前已是最新版本</strong><a v-if="release.releaseUrl" :href="release.releaseUrl" target="_blank" rel="noreferrer">查看发布说明</a><span v-if="updateMessage" class="update-message">{{ updateMessage }}</span></div><button v-if="release?.available" class="primary" :disabled="updating || !!updateMessage" @click="applyUpdate"><RefreshCw :size="17" :class="{ spinning: updating }" />{{ updateMessage ? '正在重启' : updating ? '正在升级' : '立即升级' }}</button><button v-else class="primary" :disabled="checking" @click="checkUpdate"><RefreshCw :size="17" :class="{ spinning: checking }" />{{ checking ? '正在检查' : '检查更新' }}</button></div>
      </section>
    </main>
  </div>
</template>
