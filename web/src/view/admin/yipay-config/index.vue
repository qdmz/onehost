<template>
  <div class="yipay-config-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('admin.yipayConfig.title') }}</span>
        </div>
      </template>

      <el-form ref="formRef" :model="form" label-width="160px" class="config-form" v-loading="loading">
        <el-divider content-position="left">{{ t('admin.yipayConfig.basicConfig') }}</el-divider>

        <el-form-item :label="t('admin.yipayConfig.enabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.name')" prop="name">
          <el-input v-model="form.name" style="width: 300px;" />
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.apiUrl')" prop="api_url">
          <el-input v-model="form.api_url" :placeholder="t('admin.yipayConfig.apiUrlPlaceholder')" style="width: 400px;" />
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.pid')" prop="pid">
          <el-input v-model="form.pid" :placeholder="t('admin.yipayConfig.pidPlaceholder')" style="width: 300px;" />
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.key')" prop="key">
          <el-input v-model="form.key" type="password" show-password :placeholder="t('admin.yipayConfig.keyPlaceholder')" style="width: 400px;" />
        </el-form-item>

        <el-divider content-position="left">{{ t('admin.yipayConfig.callbackConfig') }}</el-divider>

        <el-form-item :label="t('admin.yipayConfig.notifyUrl')">
          <el-input v-model="form.notify_url" readonly style="width: 500px;" />
          <div class="form-hint">{{ t('admin.yipayConfig.notifyUrlHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.returnUrl')">
          <el-input v-model="form.return_url" readonly style="width: 500px;" />
          <div class="form-hint">{{ t('admin.yipayConfig.returnUrlHint') }}</div>
        </el-form-item>

        <el-divider content-position="left">{{ t('admin.yipayConfig.feeConfig') }}</el-divider>

        <el-form-item :label="t('admin.yipayConfig.feePercent')">
          <el-input-number v-model="form.fee_percent" :min="0" :max="100" :precision="2" style="width: 200px;" />
          <span style="margin-left: 8px;">%</span>
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.minAmount')">
          <el-input-number v-model="form.min_amount" :min="0" :precision="2" style="width: 200px;" />
        </el-form-item>

        <el-form-item :label="t('admin.yipayConfig.maxAmount')">
          <el-input-number v-model="form.max_amount" :min="0" :precision="2" style="width: 200px;" />
        </el-form-item>

        <el-alert type="info" :closable="false" style="margin: 20px 0;">
          {{ t('admin.yipayConfig.hint') }}
        </el-alert>

        <div class="form-actions">
          <el-button type="primary" size="large" :loading="submitting" @click="handleSubmit">
            {{ t('common.save') }}
          </el-button>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { getYiPayConfig, updateYiPayConfig } from '@/api/admin'

const { t } = useI18n()
const formRef = ref(null)
const loading = ref(false)
const submitting = ref(false)

const form = ref({
  enabled: false,
  name: '易支付',
  api_url: '',
  pid: '',
  key: '',
  notify_url: '',
  return_url: '',
  fee_percent: 0,
  min_amount: 1,
  max_amount: 10000
})

const loadConfig = async () => {
  loading.value = true
  try {
    const res = await getYiPayConfig()
    if (res && res.code === 200 && res.data) {
      Object.assign(form.value, res.data)
    }
    // Auto-fill callback URLs based on current domain
    const origin = window.location.origin
    if (!form.value.notify_url) {
      form.value.notify_url = `${origin}/api/v1/public/payments/yipay/notify`
    }
    if (!form.value.return_url) {
      form.value.return_url = `${origin}/api/v1/public/payments/yipay/return`
    }
  } catch (error) {
    ElMessage.error(error?.message || t('admin.yipayConfig.loadFailed'))
  } finally {
    loading.value = false
  }
}

const handleSubmit = async () => {
  submitting.value = true
  try {
    const res = await updateYiPayConfig(form.value)
    if (res && res.code === 200) {
      ElMessage.success(t('admin.yipayConfig.saveSuccess'))
    } else {
      ElMessage.error(res?.message || t('admin.yipayConfig.saveFailed'))
    }
  } catch (error) {
    ElMessage.error(error?.message || t('admin.yipayConfig.saveFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped lang="scss">
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;

  > span {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-color-primary);
  }
}

.config-form {
  max-width: 800px;
  margin: 0 auto;

  :deep(.el-divider__text) {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-color-primary);
  }
}

.form-hint {
  font-size: 12px;
  color: var(--text-color-tertiary);
  margin-top: 4px;
  line-height: 1.4;
}

.form-actions {
  display: flex;
  justify-content: center;
  gap: 16px;
  padding: 24px 0 8px;
  border-top: 1px solid var(--border-color);
  margin-top: 24px;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .config-form {
    max-width: 100%;
  }

  .form-actions {
    flex-direction: column;
    align-items: center;

    .el-button {
      width: 100%;
      max-width: 200px;
    }
  }
}
</style>
