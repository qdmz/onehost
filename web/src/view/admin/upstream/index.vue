<template>
  <div class="upstream-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="header-info">
        <div class="header-title-row">
          <div class="title-icon">
            <el-icon :size="28"><Connection /></el-icon>
          </div>
          <div>
            <h2 class="page-title">{{ $t('admin.upstream.title') }}</h2>
            <p class="page-subtitle">{{ $t('admin.upstream.desc') }}</p>
          </div>
        </div>
      </div>
      <div class="header-actions">
        <el-button type="primary" :icon="Plus" round @click="openCreateDialog">
          {{ $t('admin.upstream.addProvider') }}
        </el-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row" v-if="providers.length > 0">
      <div class="stat-card">
        <div class="stat-icon stat-total">
          <el-icon :size="22"><Connection /></el-icon>
        </div>
        <div class="stat-body">
          <span class="stat-value">{{ providers.length }}</span>
          <span class="stat-label">{{ $t('admin.upstream.statTotal') }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-active">
          <el-icon :size="22"><CircleCheck /></el-icon>
        </div>
        <div class="stat-body">
          <span class="stat-value">{{ activeCount }}</span>
          <span class="stat-label">{{ $t('admin.upstream.statActive') }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-configured">
          <el-icon :size="22"><Key /></el-icon>
        </div>
        <div class="stat-body">
          <span class="stat-value">{{ configuredCount }}</span>
          <span class="stat-label">{{ $t('admin.upstream.statConfigured') }}</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-products">
          <el-icon :size="22"><Goods /></el-icon>
        </div>
        <div class="stat-body">
          <span class="stat-value">{{ syncedProducts.length }}</span>
          <span class="stat-label">{{ $t('admin.upstream.statProducts') }}</span>
        </div>
      </div>
    </div>

    <!-- 上游节点卡片列表 -->
    <div class="provider-section" v-loading="loading">
      <div class="section-header">
        <h3>{{ $t('admin.upstream.providerList') }}</h3>
      </div>

      <div v-if="providers.length === 0 && !loading" class="empty-state">
        <el-icon :size="64" color="#c0c4cc"><Connection /></el-icon>
        <p class="empty-title">{{ $t('admin.upstream.emptyTitle') }}</p>
        <p class="empty-desc">{{ $t('admin.upstream.empty') }}</p>
        <el-button type="primary" :icon="Plus" round @click="openCreateDialog" style="margin-top: 16px">
          {{ $t('admin.upstream.addProvider') }}
        </el-button>
      </div>

      <div v-else class="provider-grid">
        <div
          v-for="item in providers"
          :key="item.id"
          class="provider-card"
          :class="{ 'card-inactive': item.status !== 'active' }"
        >
          <div class="card-top">
            <div class="card-name-area">
              <div class="provider-avatar" :class="item.status === 'active' ? 'avatar-active' : 'avatar-inactive'">
                {{ item.name?.charAt(0)?.toUpperCase() || 'U' }}
              </div>
              <div class="name-body">
                <span class="provider-name">{{ item.name }}</span>
                <div class="provider-badges">
                  <el-tag :type="item.status === 'active' ? 'success' : 'info'" size="small" effect="light" round>
                    {{ item.status === 'active' ? $t('admin.upstream.active') : $t('admin.upstream.inactive') }}
                  </el-tag>
                  <el-tag :type="item.hasConfig ? 'success' : 'warning'" size="small" effect="plain" round>
                    <el-icon style="margin-right: 2px"><Key /></el-icon>
                    {{ item.hasConfig ? $t('admin.upstream.configReady') : $t('admin.upstream.configMissing') }}
                  </el-tag>
                </div>
              </div>
            </div>
          </div>
          <div class="card-info">
            <div class="info-row">
              <el-icon class="info-icon"><Location /></el-icon>
              <span class="info-label">{{ $t('admin.upstream.region') }}:</span>
              <span class="info-value">{{ item.region || '-' }}</span>
            </div>
            <div class="info-row">
              <el-icon class="info-icon"><Link /></el-icon>
              <span class="info-label">API:</span>
              <span class="info-value mono">{{ item.baseUrl || $t('admin.upstream.notSet') }}</span>
            </div>
            <div class="info-row">
              <el-icon class="info-icon"><Lock /></el-icon>
              <span class="info-label">{{ $t('admin.upstream.authType') }}:</span>
              <span class="info-value">{{ formatAuthType(item.authType) }}</span>
            </div>
          </div>
          <div class="card-actions">
            <el-button size="small" :icon="Monitor" @click="openTestDialog(item)">
              {{ $t('admin.upstream.test') }}
            </el-button>
            <el-button size="small" type="primary" :icon="Refresh" :loading="syncingId === item.id" @click="handleSync(item)">
              {{ $t('admin.upstream.syncProducts') }}
            </el-button>
            <el-button size="small" type="warning" :icon="Edit" @click="openEditDialog(item)">
              {{ $t('admin.upstream.edit') }}
            </el-button>
            <el-button size="small" type="danger" :icon="Delete" @click="handleDelete(item)" />
          </div>
        </div>
      </div>
    </div>

    <!-- 已同步产品列表 -->
    <div class="products-section" v-loading="productsLoading">
      <div class="section-header">
        <h3>{{ $t('admin.upstream.syncedProducts') }}</h3>
        <el-button size="small" :icon="Refresh" round @click="loadSyncedProducts">
          {{ $t('admin.upstream.refresh') }}
        </el-button>
      </div>
      <div v-if="syncedProducts.length === 0 && !productsLoading" class="products-empty">
        <el-empty :description="$t('admin.upstream.noProducts')" />
      </div>
      <el-table v-else :data="syncedProducts" style="width: 100%" stripe>
        <el-table-column prop="name" :label="$t('admin.upstream.productName')" min-width="160">
          <template #default="{ row }">
            <div class="product-name-cell">
              <el-icon color="#409eff"><Goods /></el-icon>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('admin.upstream.productSpec')" min-width="180">
          <template #default="{ row }">
            <div class="spec-tags">
              <el-tag size="small" type="primary" effect="plain">{{ row.cpu }} vCPU</el-tag>
              <el-tag size="small" type="success" effect="plain">{{ formatMemory(row.memory) }}</el-tag>
              <el-tag size="small" type="warning" effect="plain">{{ formatDisk(row.disk) }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('admin.upstream.bandwidth')" width="120">
          <template #default="{ row }"><span class="mono">{{ row.bandwidth }} Mbps</span></template>
        </el-table-column>
        <el-table-column :label="$t('admin.upstream.price')" width="120">
          <template #default="{ row }"><span class="price-tag">¥{{ row.price }}</span></template>
        </el-table-column>
        <el-table-column :label="$t('admin.upstream.period')" width="120">
          <template #default="{ row }">{{ row.periodType }} / {{ row.periodValue }}</template>
        </el-table-column>
        <el-table-column :label="$t('admin.upstream.status')" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" round>
              {{ row.status === 1 ? $t('admin.upstream.onSale') : $t('admin.upstream.offSale') }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<!-- 创建/编辑对话框 -->
<el-dialog
  v-model="dialogVisible"
  :title="editingId ? $t('admin.upstream.editProvider') : $t('admin.upstream.addProvider')"
  width="640px"
  destroy-on-close
  class="upstream-dialog"
>
  <el-alert
    v-if="editingId && editingConfig"
    :type="editingConfig.hasConfig ? 'success' : 'warning'"
    :closable="false"
    show-icon
    style="margin-bottom: 16px; border-radius: 8px"
  >
    <template #title>
      <span v-if="editingConfig.hasConfig">
        {{ $t('admin.upstream.configExists') }} — {{ editingConfig.authType || 'api_client' }} · {{ editingConfig.baseUrl || '***' }}
      </span>
      <span v-else>{{ $t('admin.upstream.configMissingAlert') }}</span>
    </template>
  </el-alert>
  <el-alert
    v-if="editingId"
    type="info"
    :closable="false"
    show-icon
    style="margin-bottom: 16px; border-radius: 8px"
  >
    {{ $t('admin.upstream.editHint') }}
  </el-alert>

  <el-form ref="formRef" :model="form" :rules="activeFormRules" label-width="130px" class="upstream-form">
    <el-form-item :label="$t('admin.upstream.name')" prop="name">
      <el-input v-model="form.name" :placeholder="$t('admin.upstream.namePlaceholder')" clearable />
    </el-form-item>
    <el-form-item :label="$t('admin.upstream.region')">
      <el-input v-model="form.region" :placeholder="$t('admin.upstream.regionPlaceholder')" clearable />
    </el-form-item>

    <el-divider content-position="left">
      <el-icon><Setting /></el-icon> API {{ $t('admin.upstream.config') }}
    </el-divider>

    <el-form-item :label="$t('admin.upstream.authType')" prop="authConfig.authType">
      <el-select v-model="form.authConfig.authType" style="width: 100%">
        <el-option label="api_client — API 客户端签名" value="api_client" />
        <el-option label="module — 用户名密码" value="module" />
      </el-select>
    </el-form-item>
    <el-form-item :label="$t('admin.upstream.baseUrl')" prop="authConfig.baseUrl">
      <el-input v-model="form.authConfig.baseUrl" :placeholder="$t('admin.upstream.baseUrlPlaceholder')" clearable>
        <template #prepend><el-icon><Link /></el-icon></template>
      </el-input>
    </el-form-item>

    <template v-if="form.authConfig.authType === 'api_client'">
      <el-form-item :label="$t('admin.upstream.apiId')" prop="authConfig.apiId">
        <el-input v-model="form.authConfig.apiId" clearable />
      </el-form-item>
      <el-form-item :label="$t('admin.upstream.apiKey')" prop="authConfig.apiKey">
        <el-input v-model="form.authConfig.apiKey" type="password" show-password clearable />
      </el-form-item>
    </template>
    <template v-else>
      <el-form-item :label="$t('admin.upstream.username')" prop="authConfig.username">
        <el-input v-model="form.authConfig.username" clearable />
      </el-form-item>
      <el-form-item :label="$t('admin.upstream.password')" prop="authConfig.password">
        <el-input v-model="form.authConfig.password" type="password" show-password clearable />
      </el-form-item>
    </template>

    <el-form-item :label="$t('admin.upstream.signMethod')">
      <el-select v-model="form.authConfig.signMethod" style="width: 100%">
        <el-option label="MD5" value="md5" />
        <el-option label="SHA1" value="sha1" />
        <el-option label="SHA256" value="sha256" />
      </el-select>
    </el-form-item>
    <el-form-item :label="$t('admin.upstream.timeout')">
      <el-input-number v-model="form.authConfig.timeout" :min="5" :max="120" />
      <span style="margin-left: 8px; color: #909399">s</span>
    </el-form-item>
  </el-form>

  <template #footer>
    <el-button @click="dialogVisible = false" round>{{ $t('admin.upstream.cancel') }}</el-button>
    <el-button @click="handleTestBeforeSave" :loading="testingInline" round>
      <el-icon style="margin-right: 4px"><Monitor /></el-icon>
      {{ $t('admin.upstream.testAndSave') }}
    </el-button>
    <el-button type="primary" :loading="saving" @click="handleSave" round>
      {{ $t('admin.upstream.save') }}
    </el-button>
  </template>
</el-dialog>

<!-- 测试连接对话框 -->
<el-dialog
  v-model="testDialogVisible"
  :title="$t('admin.upstream.testConnection')"
  width="480px"
  class="upstream-dialog"
>
  <div v-loading="testing" class="test-result-area">
    <el-result
      v-if="testResult !== null && testResult.ok"
      icon="success"
      :title="$t('admin.upstream.testSuccess')"
      :sub-title="testResult.msg"
    />
    <el-result
      v-else-if="testResult !== null && !testResult.ok"
      icon="error"
      :title="$t('admin.upstream.testFailed')"
      :sub-title="testResult.msg"
    />
    <el-empty v-else-if="!testing" :description="$t('admin.upstream.testHint')" />
  </div>
  <template #footer>
    <el-button @click="testDialogVisible = false" round>{{ $t('admin.upstream.close') }}</el-button>
    <el-button type="primary" :loading="testing" @click="runTest" round>
      <el-icon style="margin-right: 4px"><Monitor /></el-icon>
      {{ $t('admin.upstream.testNow') }}
    </el-button>
  </template>
</el-dialog>

<!-- 选择性同步对话框 -->
<el-dialog
  v-model="syncDialogVisible"
  :title="$t('admin.upstream.selectiveSyncTitle')"
  width="560px"
  class="upstream-dialog"
>
  <div v-loading="loadingTypes" class="sync-types-area">
    <p class="sync-hint">{{ $t('admin.upstream.selectiveSyncHint') }}</p>
    <div v-if="productTypes.length > 0" class="sync-types-list">
      <div class="sync-types-header">
        <el-checkbox
          :model-value="selectedTypes.length === productTypes.length"
          :indeterminate="selectedTypes.length > 0 && selectedTypes.length < productTypes.length"
          @change="toggleAllTypes"
        >
          {{ $t('admin.upstream.selectAll') }}
        </el-checkbox>
      </div>
      <div class="sync-type-items">
        <div v-for="pt in productTypes" :key="pt.type" class="sync-type-item">
          <el-checkbox
            :model-value="selectedTypes.includes(pt.type)"
            @change="(val) => {
              if (val) selectedTypes.push(pt.type)
              else selectedTypes = selectedTypes.filter(t => t !== pt.type)
            }"
          >
            <span class="type-name">{{ pt.name }}</span>
            <span class="type-key">({{ pt.type }})</span>
          </el-checkbox>
          <el-tag size="small" type="info" round>{{ pt.count }}</el-tag>
        </div>
      </div>
    </div>
    <el-empty v-else-if="!loadingTypes" :description="$t('admin.upstream.noProductTypes')" />
  </div>
  <template #footer>
    <el-button @click="syncDialogVisible = false" round>{{ $t('admin.upstream.cancel') }}</el-button>
    <el-button
      type="primary"
      :loading="syncLoading"
      :disabled="loadingTypes"
      @click="confirmSync"
      round
    >
      <el-icon style="margin-right: 4px"><Refresh /></el-icon>
      {{ $t('admin.upstream.confirmSync') }}
    </el-button>
  </template>
</el-dialog>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Connection, CircleCheck, Key, Goods, Location, Link, Lock,
  Monitor, Refresh, Edit, Delete, Setting
} from '@element-plus/icons-vue'
import {
  getUpstreamProviderList,
  getUpstreamProvider,
  createUpstreamProvider,
  updateUpstreamProvider,
  deleteUpstreamProvider,
  testUpstreamConnection,
  syncUpstreamProducts
} from '@/api/admin/upstream'
import { getAdminProductList } from '@/api/admin'

const loading = ref(false)
const providers = ref([])
const syncingId = ref(null)

// 选择性同步对话框
const syncDialogVisible = ref(false)
const syncProvider = ref(null)
const productTypes = ref([])
const selectedTypes = ref([])
const loadingTypes = ref(false)
const syncLoading = ref(false)
const productsLoading = ref(false)
const syncedProducts = ref([])

const dialogVisible = ref(false)
const editingId = ref(null)
const editingConfig = ref(null)
const saving = ref(false)
const testingInline = ref(false)
const formRef = ref(null)

const testDialogVisible = ref(false)
const testing = ref(false)
const testResult = ref(null)
const testProviderId = ref(null)

const defaultAuthConfig = () => ({
  authType: 'api_client',
  baseUrl: '',
  apiId: '',
  apiKey: '',
  username: '',
  password: '',
  signMethod: 'md5',
  timeout: 30
})

const form = reactive({
  name: '',
  region: '',
  authConfig: defaultAuthConfig()
})

const formRules = {
  name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }],
  'authConfig.baseUrl': [{ required: true, message: '请输入API地址', trigger: 'blur' }],
  'authConfig.apiId': [{ required: true, message: '请输入 API ID', trigger: 'blur' }],
  'authConfig.apiKey': [{ required: true, message: '请输入 API Key', trigger: 'blur' }],
  'authConfig.username': [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  'authConfig.password': [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

// 编辑模式且未填 baseUrl：不校验 API 配置（保留原有）
const activeFormRules = computed(() => {
  if (editingId.value && !form.authConfig.baseUrl) {
    return { name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }] }
  }
  if (form.authConfig.authType === 'api_client') {
    return {
      name: formRules.name,
      'authConfig.baseUrl': formRules['authConfig.baseUrl'],
      'authConfig.apiId': formRules['authConfig.apiId'],
      'authConfig.apiKey': formRules['authConfig.apiKey']
    }
  }
  return {
    name: formRules.name,
    'authConfig.baseUrl': formRules['authConfig.baseUrl'],
    'authConfig.username': formRules['authConfig.username'],
    'authConfig.password': formRules['authConfig.password']
  }
})

const activeCount = computed(() => providers.value.filter(p => p.status === 'active').length)
const configuredCount = computed(() => providers.value.filter(p => p.hasConfig).length)

const formatAuthType = (t) => {
  if (!t) return 'api_client'
  return t === 'module' ? 'module (用户名密码)' : 'api_client (签名)'
}
const formatMemory = (mb) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`
const formatDisk = (mb) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`

const loadProviders = async () => {
  loading.value = true
  try {
    const res = await getUpstreamProviderList()
    providers.value = res.data || []
  } catch (e) {
    ElMessage.error(e.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const loadSyncedProducts = async () => {
  productsLoading.value = true
  try {
    const res = await getAdminProductList({ page: 1, pageSize: 100 })
    const list = res.data?.list || res.data?.items || res.data || []
    syncedProducts.value = list.filter((p) => p.upstreamType === 'idcsmart')
  } catch (e) {
    ElMessage.error(e.message || '加载产品失败')
  } finally {
    productsLoading.value = false
  }
}

const openCreateDialog = () => {
  editingId.value = null
  editingConfig.value = null
  Object.assign(form, { name: '', region: '', authConfig: defaultAuthConfig() })
  dialogVisible.value = true
}

const openEditDialog = async (row) => {
  editingId.value = row.id
  editingConfig.value = null
  Object.assign(form, {
    name: row.name,
    region: row.region || '',
    authConfig: defaultAuthConfig()
  })
  // 从后端获取脱敏配置状态
  try {
    const res = await getUpstreamProvider(row.id)
    editingConfig.value = res.data || null
    if (editingConfig.value) {
      form.authConfig.authType = editingConfig.value.authType || 'api_client'
    }
  } catch {
    // 获取失败不阻塞编辑
  }
  dialogVisible.value = true
}

const handleSave = async () => {
  await formRef.value.validate()
  saving.value = true
  try {
    const hasNewConfig = form.authConfig.baseUrl && form.authConfig.baseUrl.trim() !== ''
    const payload = {
      name: form.name,
      region: form.region,
      authConfig: hasNewConfig ? form.authConfig : undefined
    }
    if (editingId.value) {
      await updateUpstreamProvider(editingId.value, payload)
      ElMessage.success('更新成功')
    } else {
      await createUpstreamProvider(payload)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    await loadProviders()
  } catch (e) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleTestBeforeSave = async () => {
  await formRef.value.validate()
  testingInline.value = true
  try {
    const res = await testUpstreamConnection({ authConfig: form.authConfig })
    ElMessage.success(res.msg || '连接成功')
  } catch (e) {
    ElMessage.error(e.message || '连接失败')
  } finally {
    testingInline.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(`确认删除上游节点「${row.name}」？`, '提示', { type: 'warning' })
    await deleteUpstreamProvider(row.id)
    ElMessage.success('删除成功')
    await loadProviders()
  } catch (e) {
    if (e !== 'cancel' && e?.message) ElMessage.error(e.message)
  }
}

const handleSync = async (row) => {
  // 打开选择性同步对话框
  syncProvider.value = row
  selectedTypes.value = []
  productTypes.value = []
  syncDialogVisible.value = true
  loadingTypes.value = true
  try {
    const res = await getUpstreamProductTypes(row.id)
    productTypes.value = res.data || []
    // 默认全选
    selectedTypes.value = productTypes.value.map(t => t.type)
  } catch (e) {
    ElMessage.warning('获取产品类型失败，将同步全部类型')
  } finally {
    loadingTypes.value = false
  }
}

const confirmSync = async () => {
  if (!syncProvider.value) return
  syncLoading.value = true
  syncingId.value = syncProvider.value.id
  try {
    const types = selectedTypes.value.length > 0 ? selectedTypes.value : []
    const res = await syncUpstreamProducts(syncProvider.value.id, types)
    ElMessage.success(`同步完成：新增 ${res.data?.synced ?? 0} 个，跳过 ${res.data?.skipped ?? 0} 个`)
    await loadSyncedProducts()
    syncDialogVisible.value = false
  } catch (e) {
    ElMessage.error(e.message || '同步失败')
  } finally {
    syncLoading.value = false
    syncingId.value = null
  }
}

const toggleAllTypes = (val) => {
  if (val) {
    selectedTypes.value = productTypes.value.map(t => t.type)
  } else {
    selectedTypes.value = []
  }
}

const openTestDialog = (row) => {
  testProviderId.value = row.id
  testResult.value = null
  testDialogVisible.value = true
}

const runTest = async () => {
  testing.value = true
  testResult.value = null
  try {
    await testUpstreamConnection({ providerId: testProviderId.value })
    testResult.value = { ok: true, msg: '连接成功' }
  } catch (e) {
    testResult.value = { ok: false, msg: e.message || '连接失败' }
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  loadProviders()
  loadSyncedProducts()
})
</script>

<style scoped>
.upstream-page {
  padding: 12px;
  max-width: 1400px;
  margin: 0 auto;
}

/* 页面头部 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding: 24px 28px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 16px;
  color: #fff;
  box-shadow: 0 4px 20px rgba(102, 126, 234, 0.3);
}
.header-title-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.title-icon {
  width: 52px;
  height: 52px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(10px);
}
.page-title {
  margin: 0 0 4px 0;
  font-size: 22px;
  font-weight: 700;
  color: #fff;
}
.page-subtitle {
  margin: 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.8);
  max-width: 600px;
}

/* 统计卡片 */
.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  background: var(--el-bg-color);
  border-radius: 12px;
  border: 1px solid var(--el-border-color-lighter);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  transition: transform 0.2s, box-shadow 0.2s;
}
.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
}
.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
}
.stat-total { background: linear-gradient(135deg, #667eea, #764ba2); }
.stat-active { background: linear-gradient(135deg, #43e97b, #38f9d7); }
.stat-configured { background: linear-gradient(135deg, #fa709a, #fee140); }
.stat-products { background: linear-gradient(135deg, #30cfd0, #330867); }
.stat-body { display: flex; flex-direction: column; }
.stat-value { font-size: 28px; font-weight: 700; line-height: 1.2; }
.stat-label { font-size: 13px; color: var(--el-text-color-secondary); }

/* 区块 */
.provider-section, .products-section {
  margin-bottom: 24px;
}
.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.section-header h3 {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
}

/* 空状态 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
}
.empty-title {
  margin: 16px 0 4px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}
.empty-desc {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-placeholder);
  max-width: 400px;
}

/* 节点卡片网格 */
.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}
.provider-card {
  background: var(--el-bg-color);
  border-radius: 14px;
  border: 1px solid var(--el-border-color-lighter);
  padding: 20px;
  transition: all 0.3s;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}
.provider-card:hover {
  transform: translateY(-3px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.1);
  border-color: #667eea;
}
.card-inactive {
  opacity: 0.7;
}
.card-top {
  margin-bottom: 16px;
}
.card-name-area {
  display: flex;
  align-items: center;
  gap: 12px;
}
.provider-avatar {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
}
.avatar-active { background: linear-gradient(135deg, #667eea, #764ba2); }
.avatar-inactive { background: #c0c4cc; }
.name-body { display: flex; flex-direction: column; gap: 6px; }
.provider-name { font-size: 16px; font-weight: 600; }
.provider-badges { display: flex; gap: 6px; flex-wrap: wrap; }

/* 卡片信息 */
.card-info {
  padding: 12px 0;
  border-top: 1px solid var(--el-border-color-lighter);
  border-bottom: 1px solid var(--el-border-color-lighter);
  margin-bottom: 16px;
}
.info-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  font-size: 13px;
}
.info-icon { color: var(--el-text-color-secondary); font-size: 14px; }
.info-label { color: var(--el-text-color-secondary); min-width: 60px; }
.info-value { color: var(--el-text-color-primary); }
.mono { font-family: 'Courier New', monospace; font-size: 12px; }

/* 卡片操作 */
.card-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* 产品表格 */
.product-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}
.spec-tags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.price-tag {
  color: #f56c6c;
  font-weight: 600;
}
.products-empty {
  padding: 40px 0;
}

/* 对话框 */
.upstream-dialog :deep(.el-dialog__header) {
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.upstream-form :deep(.el-divider__text) {
  font-size: 13px;
  font-weight: 600;
}
.test-result-area {
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.sync-types-area {
  min-height: 200px;
}

.sync-hint {
  color: var(--el-text-color-secondary);
  font-size: 14px;
  margin-bottom: 16px;
  line-height: 1.6;
}

.sync-types-header {
  padding: 8px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  margin-bottom: 8px;
}

.sync-type-items {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 320px;
  overflow-y: auto;
}

.sync-type-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-radius: 8px;
  transition: background 0.2s;
}

.sync-type-item:hover {
  background: var(--el-fill-color-light);
}

.sync-type-item .type-name {
  font-weight: 500;
}

.sync-type-item .type-key {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  margin-left: 4px;
}

/* 响应式 */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: 16px;
    align-items: flex-start;
  }
  .provider-grid {
    grid-template-columns: 1fr;
  }
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
