import { apiClient } from '../client'
import type { AdminUser, PaginatedResponse } from '@/types'

export interface AdminAccountFilters {
  search?: string
  role?: 'admin' | 'operator' | ''
  status?: 'active' | 'disabled' | ''
  created_from?: string
  created_to?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface CreateAdminAccountRequest {
  email: string
  password: string
  username?: string
  notes?: string
  role: 'admin' | 'operator'
  operator_pages?: string[]
  status?: 'active' | 'disabled'
}

export interface UpdateAdminAccountRequest {
  email?: string
  password?: string
  username?: string
  notes?: string
  role?: 'admin' | 'operator'
  operator_pages?: string[]
  status?: 'active' | 'disabled'
}

export async function list(
  page = 1,
  pageSize = 20,
  filters: AdminAccountFilters = {}
): Promise<PaginatedResponse<AdminUser>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminUser>>('/admin/admin-accounts', {
    params: {
      page,
      page_size: pageSize,
      search: filters.search,
      role: filters.role || undefined,
      status: filters.status || undefined,
      created_from: filters.created_from || undefined,
      created_to: filters.created_to || undefined,
      sort_by: filters.sort_by,
      sort_order: filters.sort_order
    }
  })
  return data
}

export async function create(payload: CreateAdminAccountRequest): Promise<AdminUser> {
  const { data } = await apiClient.post<AdminUser>('/admin/admin-accounts', payload)
  return data
}

export async function update(id: number, payload: UpdateAdminAccountRequest): Promise<AdminUser> {
  const { data } = await apiClient.put<AdminUser>(`/admin/admin-accounts/${id}`, payload)
  return data
}

export async function deleteAccount(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/admin-accounts/${id}`)
  return data
}

export default {
  list,
  create,
  update,
  delete: deleteAccount
}
