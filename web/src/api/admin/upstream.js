import request from '@/utils/request'

// 智简魔方(idcsmart) 上游 API 对接
// 代理销售：配置上游 API 后读取产品/价格，支付后自动开通，并可在线管理上游虚拟机

export const getUpstreamProviderList = () => {
  return request({
    url: '/v1/admin/upstream/providers',
    method: 'get'
  })
}

export const getUpstreamProvider = (id) => {
  return request({
    url: `/v1/admin/upstream/providers/${id}`,
    method: 'get'
  })
}

export const createUpstreamProvider = (data) => {
  return request({
    url: '/v1/admin/upstream/providers',
    method: 'post',
    data
  })
}

export const updateUpstreamProvider = (id, data) => {
  return request({
    url: `/v1/admin/upstream/providers/${id}`,
    method: 'put',
    data
  })
}

export const deleteUpstreamProvider = (id) => {
  return request({
    url: `/v1/admin/upstream/providers/${id}`,
    method: 'delete'
  })
}

// 测试连通性：可传 providerId 或 authConfig
export const testUpstreamConnection = (data) => {
  return request({
    url: '/v1/admin/upstream/test',
    method: 'post',
    data,
    timeout: 60000
  })
}

// 同步上游产品为可售产品
// productTypes: 可选，选择性同步 — 只同步指定类型的产品（如 ['dcimcloud', 'dcim']）
export const syncUpstreamProducts = (providerId, productTypes) => {
  const data = { providerId }
  if (productTypes && productTypes.length > 0) {
    data.productTypes = productTypes
  }
  return request({
    url: '/v1/admin/upstream/sync',
    method: 'post',
    data,
    timeout: 120000
  })
}

// 获取上游产品类型列表（用于选择性同步 UI）
export const getUpstreamProductTypes = (id) => {
  return request({
    url: `/v1/admin/upstream/providers/${id}/product-types`,
    method: 'get',
    timeout: 60000
  })
}
