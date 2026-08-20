<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  Activity,
  Bot,
  CheckCircle2,
	ClipboardPaste,
  Download,
  Eye,
  EyeOff,
  LayoutDashboard,
	Link2,
	ListChecks,
  LockKeyhole,
	LogIn,
  LogOut,
  Megaphone,
  MessageSquareReply,
  Pause,
  Pencil,
  Play,
  Plus,
  Radio,
  RefreshCw,
  Save,
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
  type TelegramSettings,
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
const showPassword = ref(false)
const authError = ref('')
const info = ref<SystemInfo | null>(null)
const overview = ref<Overview | null>(null)
const capabilities = ref<Capabilities | null>(null)
const telegramSettings = ref<TelegramSettings | null>(null)
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
const accountEntryMode = ref<'batch' | 'single'>('batch')
const accountImportText = ref('')
const accountImportErrors = ref<Array<{ line: number; error: string }>>([])
const accountEditID = ref<number | null>(null)
const accountEditForm = ref({ displayName: '', remark: '' })
const checkingAccounts = ref(false)
const authorizingAccountID = ref<number | null>(null)
const telegramCode = ref('')
const telegramPassword = ref('')
const discoveryForm = ref({ query: '', sourceType: 'public_chat' })
const campaignForm = ref({ name: '', kind: 'direct_message', target: '', message: '', runAt: '' })
const passwordForm = ref({ currentPassword: '', newPassword: '', confirmPassword: '' })
const passwordChanging = ref(false)
const telegramSettingsForm = ref({ apiId: '', apiHash: '' })
const telegramSettingsSaving = ref(false)

const versionText = computed(() => info.value?.version || 'dev')
const page = computed(() => sections.find((item) => item.id === activeSection.value) || sections[0])
const connected = computed(() => capabilities.value?.connectedAccounts || 0)
const accountStats = computed(() => ({
	total: accounts.value.length,
	online: accounts.value.filter((item) => item.status === 'online' || item.status === 'authorized' || item.status === 'restricted').length,
	working: accounts.value.filter((item) => item.status === 'logging_in' || item.status === 'checking').length,
	action: accounts.value.filter((item) => ['pending', 'password_required', 'unauthorized', 'banned', 'error', 'flood_wait'].includes(item.status)).length,
}))
let accountPollTimer: number | undefined

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
    const [systemInfo, overviewData, capabilityData, telegramSettingsData, accountData, discoveryData, campaignData, relayData] = await Promise.all([
      api.systemInfo(), api.overview(), api.capabilities(), api.telegramSettings(), api.accounts(), api.discovery(), api.campaigns(), api.relay(),
    ])
    info.value = systemInfo
    overview.value = overviewData
    capabilities.value = capabilityData
    telegramSettings.value = telegramSettingsData
    telegramSettingsForm.value.apiId = telegramSettingsData.apiId > 0 ? String(telegramSettingsData.apiId) : ''
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
  authSubmitting.value = true
  try {
    if (auth.value?.configured) await api.login(password.value)
    else await api.setup(auth.value?.defaultPassword || 'admin')
    auth.value = { configured: true, authenticated: true, defaultPassword: '' }
    password.value = ''
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
    auth.value = { configured: true, authenticated: false, defaultPassword: '' }
    info.value = null
    overview.value = null
  }
}

async function changeAdminPassword() {
  clearNotices()
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    error.value = '两次输入的新密码不一致'
    return
  }
  passwordChanging.value = true
  try {
    await api.changePassword(passwordForm.value)
    passwordForm.value = { currentPassword: '', newPassword: '', confirmPassword: '' }
    success.value = '管理员密码已更新。'
  } catch (err) {
    error.value = messageFrom(err, '修改密码失败')
  } finally {
    passwordChanging.value = false
  }
}

async function saveTelegramSettings() {
  telegramSettingsSaving.value = true
  clearNotices()
  try {
    await api.updateTelegramSettings({ apiId: Number(telegramSettingsForm.value.apiId), apiHash: telegramSettingsForm.value.apiHash.trim() })
    const [settingsData, capabilityData] = await Promise.all([api.telegramSettings(), api.capabilities()])
    telegramSettings.value = settingsData
    capabilities.value = capabilityData
    telegramSettingsForm.value.apiHash = ''
    const pendingAccounts = accounts.value.filter((item) => item.hasCodeUrl && ['pending', 'error', 'unauthorized', 'flood_wait'].includes(item.status))
    const started = await Promise.allSettled(pendingAccounts.map((item) => api.autoLoginAccount(item.id)))
    const startedCount = started.filter((result) => result.status === 'fulfilled').length
    accounts.value = await api.accounts()
    success.value = startedCount > 0 ? `Telegram API 配置已保存，并自动开始登录 ${startedCount} 个账号。` : 'Telegram API 配置已保存并立即生效。'
  } catch (err) {
    error.value = messageFrom(err, '保存 Telegram API 配置失败')
  } finally {
    telegramSettingsSaving.value = false
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

async function openAccountEdit(item: TelegramAccount) {
	const table = window.document.querySelector('.table-wrap')
	accountEditID.value = item.id
	accountEditForm.value = { displayName: item.displayName, remark: item.remark }
	clearNotices()
	await nextTick()
	if (window.innerWidth <= 620 && table) table.scrollLeft = 0
}

function closeAccountEdit() {
	accountEditID.value = null
	accountEditForm.value = { displayName: '', remark: '' }
}

async function saveAccountEdit(item: TelegramAccount) {
	saving.value = `account-edit-${item.id}`
	clearNotices()
	try {
		await api.updateAccount(item.id, accountEditForm.value)
		accounts.value = await api.accounts()
		closeAccountEdit()
		success.value = '账号昵称和备注已保存。'
	} catch (err) {
		error.value = messageFrom(err, '保存账号信息失败')
	} finally {
		saving.value = ''
	}
}

async function importAccounts() {
	saving.value = 'account-import'
	accountImportErrors.value = []
	clearNotices()
	try {
		const result = await api.importAccounts(accountImportText.value)
		accountImportErrors.value = result.errors
		accountImportText.value = ''
		accounts.value = await api.accounts()
		overview.value = await api.overview()
		if (result.autoLoginQueued > 0) {
			success.value = `已导入 ${result.added} 个账号，并自动开始登录 ${result.autoLoginQueued} 个账号。`
		} else if (result.autoLoginBlocked) {
			error.value = '账号已导入，但未自动登录：请先配置 Telegram API ID 和 API Hash。'
		} else {
			success.value = `已导入 ${result.added} 个账号，更新 ${result.updated} 个，跳过 ${result.skipped} 个。`
		}
	} catch (err) {
		error.value = messageFrom(err, '批量导入失败')
	} finally {
		saving.value = ''
	}
}

async function autoLoginAccount(item: TelegramAccount) {
	saving.value = `auto-${item.id}`
	clearNotices()
	try {
		await api.autoLoginAccount(item.id)
		accounts.value = await api.accounts()
		success.value = '自动登录已开始，系统正在等待新的 Telegram 验证码。'
	} catch (err) {
		accounts.value = await api.accounts().catch(() => accounts.value)
		error.value = messageFrom(err, '启动自动登录失败')
	} finally {
		saving.value = ''
	}
}

async function checkAccount(item: TelegramAccount) {
	saving.value = `check-${item.id}`
	clearNotices()
	try {
		await api.checkAccount(item.id)
		accounts.value = await api.accounts()
		success.value = '账号状态检测已开始。'
	} catch (err) {
		error.value = messageFrom(err, '启动状态检测失败')
	} finally {
		saving.value = ''
	}
}

async function checkAllAccounts() {
	checkingAccounts.value = true
	clearNotices()
	try {
		const result = await api.checkAllAccounts()
		accounts.value = await api.accounts()
		success.value = result.queued ? `已开始检测 ${result.queued} 个账号。` : '当前没有可检测的已登录账号。'
	} catch (err) {
		error.value = messageFrom(err, '启动批量检测失败')
	} finally {
		checkingAccounts.value = false
	}
}

async function pollAccountJobs() {
	if (!auth.value?.authenticated || activeSection.value !== 'accounts') return
	if (!accounts.value.some((item) => item.status === 'logging_in' || item.status === 'checking')) return
	try {
		const [accountData, capabilityData, overviewData] = await Promise.all([api.accounts(), api.capabilities(), api.overview()])
		accounts.value = accountData
		capabilities.value = capabilityData
		overview.value = overviewData
	} catch {
		// The next interval retries; foreground notices remain untouched.
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
	return ({ pending: '待登录', logging_in: '自动登录中', checking: '检测中', code_sent: '等待验证码', password_required: '等待 2FA', authorized: '已授权', online: '在线', restricted: '受限', unauthorized: '会话失效', banned: '已封禁', flood_wait: '请求受限', offline: '离线', error: '检测失败', draft: '草稿', paused: '已暂停', pending_connection: '等待账号连接', ready: '就绪', running: '执行中', completed: '已完成', failed: '失败' } as Record<string, string>)[value] || value
}

function accountStatusTone(value: string) {
	if (value === 'online') return 'ok'
	if (value === 'logging_in' || value === 'checking') return 'working'
	if (value === 'restricted' || value === 'password_required' || value === 'flood_wait') return 'warning'
	if (value === 'unauthorized' || value === 'banned' || value === 'error') return 'danger'
	return 'muted'
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
	accountPollTimer = window.setInterval(() => void pollAccountJobs(), 4000)
  void bootstrap()
})

onBeforeUnmount(() => {
	window.removeEventListener('hashchange', syncSection)
	if (accountPollTimer) window.clearInterval(accountPollTimer)
})
</script>

<template>
  <div v-if="authLoading" class="auth-loading" aria-label="正在加载">
    <Bot :size="30" /><RefreshCw :size="20" class="spinning" />
  </div>

  <main v-else-if="!auth?.authenticated" class="auth-page">
    <section class="auth-copy">
      <div class="auth-brand"><Bot :size="28" /><span>Teleflow</span></div>
      <div><p class="auth-kicker">私域运营控制台</p><h1>{{ auth?.configured ? '欢迎回来' : '初始化管理员' }}</h1><p>{{ auth?.configured ? '登录后管理账号矩阵、营销任务与消息中转。' : '使用默认管理员密码进入系统，登录后可在系统设置中修改。' }}</p></div>
      <small>单一所有者模式 · 本地数据存储</small>
    </section>
    <section class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="submitAuth">
        <div class="form-heading"><LockKeyhole :size="21" /><div><h2>{{ auth?.configured ? '管理员登录' : '首次登录' }}</h2><p>{{ auth?.configured ? '请输入当前实例的管理员密码' : '登录后请及时修改默认密码' }}</p></div></div>
        <template v-if="auth?.configured"><label for="password">管理员密码</label><div class="password-field">
          <input id="password" v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="current-password" required autofocus />
          <button type="button" :title="showPassword ? '隐藏密码' : '显示密码'" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword"><EyeOff v-if="showPassword" :size="18" /><Eye v-else :size="18" /></button>
        </div></template>
        <div v-else class="default-password"><span>默认管理员密码</span><strong>{{ auth?.defaultPassword || 'admin' }}</strong></div>
        <p v-if="authError" class="auth-error">{{ authError }}</p>
        <button class="primary auth-submit" :disabled="authSubmitting" type="submit"><RefreshCw v-if="authSubmitting" :size="17" class="spinning" /><LockKeyhole v-else :size="17" />{{ authSubmitting ? '请稍候' : auth?.configured ? '登录' : '使用默认密码进入' }}</button>
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
        <div class="capability-banner"><Smartphone :size="19" /><div><strong>{{ capabilities?.telegramConfigured ? 'Telegram API 已配置，导入后自动登录' : '未配置 Telegram API，无法登录账号' }}</strong><p>{{ capabilities?.telegramConfigured ? '导入带取码链接的账号后，系统会自动获取验证码并登录；需要两步验证时会停在“等待 2FA”。' : 'Telegram API ID 和 API Hash 是 Telegram 官方签发的客户端凭据，不能由系统自动生成。' }}</p><a v-if="!capabilities?.telegramConfigured" href="#settings">前往系统设置</a></div></div>
        <div class="account-status-strip" aria-label="账号状态统计">
          <div><span>全部账号</span><strong>{{ accountStats.total }}</strong></div>
          <div><span>当前在线</span><strong class="tone-good">{{ accountStats.online }}</strong></div>
          <div><span>处理中</span><strong class="tone-working">{{ accountStats.working }}</strong></div>
          <div><span>需要处理</span><strong class="tone-action">{{ accountStats.action }}</strong></div>
        </div>
        <div class="account-workspace-grid">
          <section class="workspace-panel account-import-panel">
            <div class="section-heading"><div><h2>批量导入账号</h2><p>每行一个：手机号----取码链接，也支持 |、Tab、逗号和分号。</p></div><ClipboardPaste :size="20" /></div>
            <div class="segmented" role="tablist" aria-label="账号录入方式"><button type="button" :class="{ active: accountEntryMode === 'batch' }" @click="accountEntryMode = 'batch'"><ListChecks :size="15" />批量导入</button><button type="button" :class="{ active: accountEntryMode === 'single' }" @click="accountEntryMode = 'single'"><Plus :size="15" />手动登记</button></div>
            <form v-if="accountEntryMode === 'batch'" class="account-import-form" @submit.prevent="importAccounts"><label>账号清单<textarea v-model="accountImportText" rows="7" placeholder="+1xxxxxxxxxx----https://vendor.example/…/GetHTML&#10;+86xxxxxxxxxxx----https://vendor.example/…/GetHTML" required></textarea></label><div class="form-actions"><span>支持批次内重复导入，系统会自动更新链接</span><button class="primary" :disabled="saving === 'account-import'"><Download :size="16" />{{ saving === 'account-import' ? '导入中' : '导入账号' }}</button></div></form>
            <form v-else class="form-grid account-form" @submit.prevent="createAccount"><label>手机号<input v-model="accountForm.phone" placeholder="+8613800138000" autocomplete="tel" required /></label><label>账号名称<input v-model="accountForm.displayName" placeholder="例如：获客账号 A" maxlength="80" /></label><button class="primary" :disabled="saving === 'account'"><Plus :size="17" />{{ saving === 'account' ? '添加中' : '登记账号' }}</button></form>
            <div v-if="accountImportErrors.length" class="import-errors"><strong>{{ accountImportErrors.length }} 行未导入</strong><span v-for="problem in accountImportErrors" :key="problem.line">第 {{ problem.line }} 行：{{ problem.error }}</span></div>
          </section>
          <section class="workspace-panel account-actions-panel"><div class="section-heading"><div><h2>连接检查</h2><p>只检测已有加密会话的账号</p></div><Link2 :size="20" /></div><button class="secondary wide" :disabled="checkingAccounts || !accounts.some((item) => item.hasSession)" @click="checkAllAccounts"><RefreshCw :size="16" :class="{ spinning: checkingAccounts }" />{{ checkingAccounts ? '正在安排检测' : '检测全部已登录账号' }}</button><div class="action-note"><CheckCircle2 :size="16" /><span>在线表示最近一次 Telegram API 检查成功；受限和会话失效会单独标色。</span></div></section>
        </div>
        <div class="workspace-panel">
          <div class="section-heading"><div><h2>账号列表</h2><p>最近检测时间：后台任务完成后自动刷新</p></div><span class="status muted">{{ accounts.length }} 个账号</span></div>
          <div v-if="accounts.length" class="table-wrap">
            <table>
              <thead><tr><th>账号</th><th>手机号</th><th>状态</th><th>最近活动</th><th class="account-actions">操作</th></tr></thead>
              <tbody>
                <template v-for="item in accounts" :key="item.id">
                  <tr>
                    <td><strong>{{ item.displayName || '未命名账号' }}</strong><small v-if="item.remark" class="account-remark">{{ item.remark }}</small><small v-if="item.username" class="account-username">@{{ item.username }}</small><small v-if="item.lastError" class="account-error">{{ item.lastError }}</small></td>
                    <td>{{ item.phone }}</td>
                    <td><span class="status" :class="accountStatusTone(item.status)"><span class="status-dot-inline"></span>{{ statusText(item.status) }}</span><small v-if="item.lastCheckedAt" class="account-username">检查于 {{ formatDate(item.lastCheckedAt) }}</small></td>
                    <td>{{ formatDate(item.lastSeenAt || item.createdAt) }}</td>
                    <td class="account-actions">
                      <button class="secondary compact" :disabled="saving === `account-edit-${item.id}`" title="编辑账号昵称和备注" @click="openAccountEdit(item)"><Pencil :size="15" />编辑</button>
                      <button v-if="item.hasCodeUrl && ['pending', 'error', 'unauthorized', 'flood_wait'].includes(item.status)" class="secondary compact" :disabled="saving === `auto-${item.id}` || !capabilities?.telegramConfigured" title="使用取码链接自动登录" @click="autoLoginAccount(item)"><LogIn :size="15" />{{ saving === `auto-${item.id}` ? '启动中' : '自动登录' }}</button>
                      <button v-if="item.hasSession && !['logging_in', 'checking'].includes(item.status)" class="secondary compact" :disabled="saving === `check-${item.id}`" title="检测账号当前存活状态" @click="checkAccount(item)"><RefreshCw :size="15" />检测</button>
                      <button v-if="item.status === 'pending' || item.status === 'offline' || item.status === 'error'" class="secondary compact" :disabled="saving === `auth-${item.id}` || !capabilities?.telegramConfigured" title="发送 Telegram 验证码" @click="requestTelegramCode(item)"><Smartphone :size="15" />手动登录</button>
                      <button v-else-if="item.status === 'code_sent'" class="secondary compact" @click="openAccountAuthorization(item)">输入验证码</button>
                      <button v-else-if="item.status === 'password_required'" class="secondary compact" @click="openAccountAuthorization(item)">输入 2FA</button>
                      <button class="icon-button danger" title="删除账号" :disabled="saving === `account-${item.id}`" @click="deleteAccount(item.id)"><Trash2 :size="16" /></button>
                    </td>
                  </tr>
                  <tr v-if="accountEditID === item.id" class="account-edit-row">
                    <td colspan="5">
                      <form class="account-edit-form" @submit.prevent="saveAccountEdit(item)">
                        <label>账号昵称<input v-model="accountEditForm.displayName" maxlength="80" placeholder="例如：获客账号 A" /></label>
                        <label>备注<textarea v-model="accountEditForm.remark" maxlength="500" rows="2" placeholder="记录地区、用途或负责人等信息"></textarea></label>
                        <div class="account-edit-actions"><button type="button" class="secondary compact" @click="closeAccountEdit">取消</button><button type="submit" class="primary compact" :disabled="saving === `account-edit-${item.id}`"><Save :size="15" />{{ saving === `account-edit-${item.id}` ? '保存中' : '保存' }}</button></div>
                      </form>
                    </td>
                  </tr>
                  <tr v-if="authorizingAccountID === item.id && (item.status === 'code_sent' || item.status === 'password_required')" class="auth-step-row">
                    <td colspan="5">
                      <form v-if="item.status === 'code_sent'" class="account-auth-step" @submit.prevent="verifyTelegramCode(item)"><div><strong>输入 Telegram 验证码</strong><p>验证码发送于 {{ formatDate(item.codeSentAt) }}</p></div><input v-model="telegramCode" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{3,8}" placeholder="验证码" required /><button class="primary" :disabled="saving === `auth-${item.id}`"><ShieldCheck :size="16" />验证</button></form>
                      <form v-else class="account-auth-step" @submit.prevent="verifyTelegramPassword(item)"><div><strong>输入两步验证密码</strong><p>密码只用于本次 Telegram SRP 验证，不会保存。</p></div><input v-model="telegramPassword" type="password" autocomplete="current-password" placeholder="2FA 密码" maxlength="256" required /><button class="primary" :disabled="saving === `auth-${item.id}`"><ShieldCheck :size="16" />完成授权</button></form>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>
          <div v-else class="empty"><Users :size="28" /><p>还没有登记矩阵账号</p></div>
        </div>
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
        <form class="panel telegram-setup-panel" @submit.prevent="saveTelegramSettings"><div class="panel-title"><div><h2>Telegram 登录配置</h2><p>导入账号自动登录所需</p></div><Smartphone :size="20" /></div><div class="setup-status"><span class="status" :class="capabilities?.telegramConfigured ? 'ok' : 'danger'">{{ capabilities?.telegramConfigured ? '已配置' : '未配置' }}</span><strong>{{ capabilities?.telegramConfigured ? '导入后会自动登录' : '当前无法登录 Telegram 账号' }}</strong></div><div class="password-settings-fields"><label>API ID<input v-model="telegramSettingsForm.apiId" inputmode="numeric" pattern="[0-9]+" placeholder="12345678" required /></label><label>API Hash<input v-model="telegramSettingsForm.apiHash" type="password" autocomplete="new-password" pattern="[A-Fa-f0-9]{32}" :placeholder="telegramSettings?.hasApiHash ? '重新输入以更新' : '32 位 API Hash'" required /></label></div><p class="setup-copy">这两项由 Telegram 官方签发，不能自动生成。前往 <a href="https://my.telegram.org/apps" target="_blank" rel="noreferrer">my.telegram.org/apps</a> 创建应用后填入；API Hash 会加密保存且不会回显。</p><button class="primary" type="submit" :disabled="telegramSettingsSaving"><RefreshCw v-if="telegramSettingsSaving" :size="17" class="spinning" /><ShieldCheck v-else :size="17" />{{ telegramSettingsSaving ? '正在保存' : '保存 Telegram 配置' }}</button></form>
        <div class="panel update-panel"><div class="panel-title"><div><h2>版本升级</h2><p>从 GitHub Releases 获取稳定版本</p></div><ShieldCheck :size="20" /></div><div v-if="!release" class="update-empty"><Download :size="28" /><p>检查是否有可用的新版本</p></div><div v-else class="release-result"><strong v-if="!release.configured">尚未配置 GitHub 仓库</strong><strong v-else-if="release.available">发现新版本 {{ release.latestVersion }}</strong><strong v-else>当前已是最新版本</strong><a v-if="release.releaseUrl" :href="release.releaseUrl" target="_blank" rel="noreferrer">查看发布说明</a><span v-if="updateMessage" class="update-message">{{ updateMessage }}</span></div><button v-if="release?.available" class="primary" :disabled="updating || !!updateMessage" @click="applyUpdate"><RefreshCw :size="17" :class="{ spinning: updating }" />{{ updateMessage ? '正在重启' : updating ? '正在升级' : '立即升级' }}</button><button v-else class="primary" :disabled="checking" @click="checkUpdate"><RefreshCw :size="17" :class="{ spinning: checking }" />{{ checking ? '正在检查' : '检查更新' }}</button></div>
        <form class="panel password-settings" @submit.prevent="changeAdminPassword"><div class="panel-title"><div><h2>管理员密码</h2><p>更新此实例的登录凭据</p></div><LockKeyhole :size="20" /></div><div class="password-settings-fields"><label>当前密码<input v-model="passwordForm.currentPassword" type="password" autocomplete="current-password" required /></label><label>新密码<input v-model="passwordForm.newPassword" type="password" autocomplete="new-password" minlength="8" maxlength="128" required /></label><label>确认新密码<input v-model="passwordForm.confirmPassword" type="password" autocomplete="new-password" minlength="8" maxlength="128" required /></label></div><button class="primary" :disabled="passwordChanging" type="submit"><RefreshCw v-if="passwordChanging" :size="17" class="spinning" /><ShieldCheck v-else :size="17" />{{ passwordChanging ? '正在保存' : '修改密码' }}</button></form>
      </section>
    </main>
  </div>
</template>
