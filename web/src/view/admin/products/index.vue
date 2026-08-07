<template>
  <div class="products-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ t('admin.products.title') }}</span>
          <el-button type="primary" @click="handleAdd">
            <el-icon><Plus /></el-icon>
            {{ t('admin.products.addProduct') }}
          </el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <div class="toolbar">
        <el-input
          v-model="searchKeyword"
          :placeholder="t('admin.products.searchPlaceholder')"
          style="width: 250px;"
          clearable
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select
          v-model="searchStatus"
          :placeholder="t('admin.products.selectStatus')"
          style="width: 150px; margin-left: 10px;"
          clearable
        >
          <el-option :label="t('admin.products.all')" value="" />
          <el-option :label="t('admin.products.onShelf')" :value="1" />
          <el-option :label="t('admin.products.offShelf')" :value="0" />
        </el-select>
        <el-button type="primary" style="margin-left: 10px;" @click="handleSearch">
          {{ t('common.search') }}
        </el-button>
        <el-button style="margin-left: 10px;" @click="resetFilters">
          {{ t('common.reset') }}
        </el-button>
      </div>

      <!-- 产品表格 -->
      <el-table v-loading="loading" :data="productList" stripe style="width: 100%">
        <el-table-column :label="t('admin.products.id')" prop="id" width="80" />
        <el-table-column :label="t('admin.products.name')" prop="name" min-width="150" />
        <el-table-column :label="t('admin.products.type')" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ getTypeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.products.specs')" min-width="200">
          <template #default="{ row }">
            {{ row.cpu }}核 / {{ formatMemory(row.memory) }} / {{ formatDisk(row.disk) }}
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.products.price')" width="120">
          <template #default="{ row }">
            <span class="price-text">¥{{ row.price }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.products.status')" width="100">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 1"
              @change="(val) => toggleStatus(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.products.actions')" width="180" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" size="small" @click="handleEdit(row)">
              {{ t('common.edit') }}
            </el-button>
            <el-button type="danger" size="small" @click="handleDelete(row)">
              {{ t('common.delete') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div v-if="total > pageSize" class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 产品表单弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? t('admin.products.editProduct') : t('admin.products.addProduct')"
      width="700px"
      :close-on-click-modal="false"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="100px"
      >
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item :label="t('admin.products.name')" prop="name">
              <el-input v-model="form.name" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('admin.products.type')" prop="type">
              <el-select v-model="form.type" style="width: 100%;">
                <el-option :label="t('admin.products.typeVM')" value="vm" />
                <el-option :label="t('admin.products.typeContainer')" value="container" />
                <el-option :label="t('admin.products.typeGPU')" value="gpu" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item :label="t('admin.products.description')" prop="description">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>

        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="t('admin.products.cpu')" prop="cpu">
              <el-input-number v-model="form.cpu" :min="1" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="t('admin.products.memory')" prop="memory">
              <el-input-number v-model="form.memory" :min="128" :step="128" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="t('admin.products.disk')" prop="disk">
              <el-input-number v-model="form.disk" :min="1024" :step="1024" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="t('admin.products.bandwidth')" prop="bandwidth">
              <el-input-number v-model="form.bandwidth" :min="1" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="t('admin.products.traffic')" prop="traffic">
              <el-input-number v-model="form.traffic" :min="0" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="t('admin.products.price')" prop="price">
              <el-input-number v-model="form.price" :min="0" :precision="2" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item :label="t('admin.products.periodType')" prop="periodType">
              <el-select v-model="form.periodType" style="width: 100%;">
                <el-option :label="t('admin.products.periodTypeMonth')" value="month" />
                <el-option :label="t('admin.products.periodTypeDay')" value="day" />
                <el-option :label="t('admin.products.periodTypeHour')" value="hour" />
                <el-option :label="t('admin.products.periodTypeYear')" value="year" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('admin.products.periodValue')" prop="periodValue">
              <el-input-number v-model="form.periodValue" :min="1" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item :label="t('admin.products.category')" prop="category">
          <el-input v-model="form.category" :placeholder="t('admin.products.categoryPlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('admin.products.providerIds')" prop="providerIds">
          <el-input v-model="form.providerIds" :placeholder="t('admin.products.providerIdsPlaceholder')" />
        </el-form-item>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item :label="t('admin.products.stock')" prop="stock">
              <el-input-number v-model="form.stock" :min="-1" :step="1" />
              <div style="color: #999; font-size: 12px;">{{ t('admin.products.stockHint') }}</div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="t('admin.products.maxPerUser')" prop="maxPerUser">
              <el-input-number v-model="form.maxPerUser" :min="0" :step="1" />
              <div style="color: #999; font-size: 12px;">{{ t('admin.products.maxPerUserHint') }}</div>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { formatMemorySize, formatDiskSize } from '@/utils/unit-formatter'
import { getAdminProductList, createAdminProduct, updateAdminProduct, deleteAdminProduct, updateAdminProductStatus } from '@/api/admin'

const { t } = useI18n()

const loading = ref(true)
const productList = ref([])
const searchKeyword = ref('')
const searchStatus = ref('')
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)
const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const formRef = ref(null)

const form = ref({
  name: '',
  type: 'vm',
  description: '',
  category: '',
  cpu: 1,
  memory: 1024,
  disk: 10240,
  bandwidth: 100,
  traffic: 0,
  price: 1,
  periodType: 'month',
  periodValue: 1,
  providerIds: '',
  imageIds: '',
  stock: -1,
  maxPerUser: 0
})

const formRules = {
  name: [{ required: true, message: t('admin.products.nameRequired'), trigger: 'blur' }],
  type: [{ required: true, message: t('admin.products.typeRequired'), trigger: 'change' }],
  cpu: [{ required: true, message: t('admin.products.cpuRequired'), trigger: 'blur' }],
  memory: [{ required: true, message: t('admin.products.memoryRequired'), trigger: 'blur' }],
  disk: [{ required: true, message: t('admin.products.diskRequired'), trigger: 'blur' }],
  price: [{ required: true, message: t('admin.products.priceRequired'), trigger: 'blur' }],
  periodType: [{ required: true, message: t('admin.products.periodTypeRequired'), trigger: 'change' }],
  periodValue: [{ required: true, message: t('admin.products.periodValueRequired'), trigger: 'blur' }]
}

const formatMemory = (memory) => formatMemorySize(memory)
const formatDisk = (disk) => formatDiskSize(disk)

// 获取产品类型中文标签
const getTypeLabel = (type) => {
  const typeMap = {
    vm: t('admin.products.typeVM'),
    container: t('admin.products.typeContainer'),
    gpu: t('admin.products.typeGPU')
  }
  return typeMap[type] || type
}

// 模拟加载产品列表(管理员端API需后端配合，这里使用product.js中的列表API作为基础)
const loadProducts = async () => {
  loading.value = true
  try {
    const params = {
      page: currentPage.value,
      pageSize: pageSize.value,
      keyword: searchKeyword.value || undefined,
      status: searchStatus.value === '' ? undefined : searchStatus.value
    }
    const res = await getAdminProductList(params)
    if (res.code === 200) {
      productList.value = res.data?.list || res.data?.items || []
      total.value = res.data?.total || 0
    }
  } catch (error) {
    console.error('加载产品列表失败:', error)
    ElMessage.error(error?.message || t('admin.products.loadFailed'))
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  currentPage.value = 1
  loadProducts()
}

const resetFilters = () => {
  searchKeyword.value = ''
  searchStatus.value = ''
  currentPage.value = 1
  loadProducts()
}

const handlePageChange = (page) => {
  currentPage.value = page
  loadProducts()
}

const handleSizeChange = (size) => {
  pageSize.value = size
  currentPage.value = 1
  loadProducts()
}

// 新增
const handleAdd = () => {
  isEdit.value = false
  form.value = {
    name: '',
    type: 'vm',
    description: '',
    category: '',
    cpu: 1,
    memory: 1024,
    disk: 10240,
    bandwidth: 100,
    traffic: 0,
    price: 1,
    periodType: 'month',
    periodValue: 1,
    providerIds: '',
    imageIds: '',
    stock: -1,
    maxPerUser: 0
  }
  dialogVisible.value = true
}

// 编辑
const handleEdit = (row) => {
  isEdit.value = true
  form.value = {
    ...row,
    category: row.category || '',
    periodType: row.periodType || 'month',
    periodValue: row.periodValue || 1,
    providerIds: row.providerIds || '',
    stock: row.stock !== undefined && row.stock !== null ? row.stock : -1,
    maxPerUser: row.maxPerUser !== undefined && row.maxPerUser !== null ? row.maxPerUser : 0
  }
  dialogVisible.value = true
}

// 提交
const handleSubmit = async () => {
  try {
    const valid = await formRef.value?.validate()
    if (!valid) return
  } catch {
    ElMessage.warning(t('admin.products.validationFailed'))
    return
  }

  submitting.value = true
  try {
    let res
    if (isEdit.value) {
      res = await updateAdminProduct(form.value.id, form.value)
    } else {
      res = await createAdminProduct(form.value)
    }
    if (res.code === 200) {
      ElMessage.success(isEdit.value ? t('admin.products.updateSuccess') : t('admin.products.createSuccess'))
      dialogVisible.value = false
      loadProducts()
    } else {
      ElMessage.error(res.message || t('admin.products.saveFailed'))
    }
  } catch (error) {
    ElMessage.error(error?.message || t('admin.products.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// 上下架
const toggleStatus = async (row, val) => {
  try {
    await updateAdminProductStatus(row.id, { status: val ? 1 : 0 })
    // 更新本地状态
    row.status = val ? 1 : 0
    ElMessage.success(val ? t('admin.products.onShelfSuccess') : t('admin.products.offShelfSuccess'))
  } catch (error) {
    ElMessage.error(error?.message || t('admin.products.statusChangeFailed'))
  }
}

// 删除
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      t('admin.products.confirmDelete', { name: row.name }),
      t('common.tip'),
      { confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
    await deleteAdminProduct(row.id)
    ElMessage.success(t('admin.products.deleteSuccess'))
    loadProducts()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error?.message || t('admin.products.deleteFailed'))
    }
  }
}

onMounted(() => {
  loadProducts()
})
</script>

<style lang="scss" scoped>
.products-container {
  padding: 24px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
}

.toolbar {
  margin-bottom: 20px;
}

.price-text {
  color: #f56c6c;
  font-weight: 600;
}

.pagination-wrapper {
  margin-top: 24px;
  display: flex;
  justify-content: flex-end;
}
</style>
