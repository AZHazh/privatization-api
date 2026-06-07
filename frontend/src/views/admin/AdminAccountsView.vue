<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full md:w-72">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="filters.search"
              type="text"
              class="input pl-10"
              :placeholder="t('admin.adminAccounts.searchPlaceholder')"
              @keyup.enter="loadAccounts"
            />
          </div>
          <select v-model="filters.role" class="input w-full sm:w-36" @change="loadAccounts">
            <option value="">{{ t('admin.adminAccounts.allRoles') }}</option>
            <option value="admin">{{ t('admin.adminAccounts.admin') }}</option>
            <option value="operator">{{ t('admin.adminAccounts.operator') }}</option>
          </select>
          <select v-model="filters.status" class="input w-full sm:w-36" @change="loadAccounts">
            <option value="">{{ t('admin.adminAccounts.allStatus') }}</option>
            <option value="active">{{ t('common.active') }}</option>
            <option value="disabled">{{ t('admin.users.disabled') }}</option>
          </select>
          <input
            v-model="filters.created_from"
            type="date"
            class="input w-full sm:w-40"
            :title="t('admin.adminAccounts.createdFrom')"
            @change="loadAccounts"
          />
          <input
            v-model="filters.created_to"
            type="date"
            class="input w-full sm:w-40"
            :title="t('admin.adminAccounts.createdTo')"
            @change="loadAccounts"
          />
          <button class="btn btn-secondary" :disabled="loading" @click="loadAccounts">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
          <button class="btn btn-primary ml-auto" @click="openCreate">
            <Icon name="plus" size="md" />
            {{ t('admin.adminAccounts.create') }}
          </button>
        </div>
      </template>

      <template #table>
        <div class="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>{{ t('admin.users.username') }}</th>
                <th>{{ t('admin.users.email') }}</th>
                <th>{{ t('admin.adminAccounts.role') }}</th>
                <th>{{ t('admin.adminAccounts.pages') }}</th>
                <th>{{ t('common.status') }}</th>
                <th>{{ t('admin.users.createdAt') }}</th>
                <th>{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loading">
                <td colspan="7" class="text-center">{{ t('common.loading') }}</td>
              </tr>
              <tr v-else-if="accounts.length === 0">
                <td colspan="7" class="text-center text-gray-500">{{ t('admin.adminAccounts.empty') }}</td>
              </tr>
              <tr v-for="account in accounts" v-else :key="account.id">
                <td>{{ account.username || '-' }}</td>
                <td>{{ account.email }}</td>
                <td>
                  <span class="badge" :class="account.role === 'admin' ? 'badge-primary' : 'badge-secondary'">
                    {{ roleLabel(account.role) }}
                  </span>
                </td>
                <td class="max-w-md">
                  <span v-if="account.role === 'admin'">{{ t('admin.adminAccounts.allPages') }}</span>
                  <span v-else>{{ pageSummary(account.operator_pages) }}</span>
                </td>
                <td>
                  <span class="badge" :class="account.status === 'active' ? 'badge-success' : 'badge-warning'">
                    {{ account.status === 'active' ? t('common.active') : t('admin.users.disabled') }}
                  </span>
                </td>
                <td>{{ formatDate(account.created_at) }}</td>
                <td>
                  <div class="flex items-center gap-2">
                    <button class="btn btn-secondary btn-sm" @click="openEdit(account)">
                      {{ t('common.edit') }}
                    </button>
                    <button
                      class="btn btn-danger btn-sm"
                      :disabled="account.role === 'admin'"
                      @click="deleteAccount(account)"
                    >
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>

    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="w-full max-w-2xl rounded-lg bg-white p-6 shadow-xl dark:bg-dark-800">
        <div class="mb-5 flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ editing ? t('admin.adminAccounts.edit') : t('admin.adminAccounts.create') }}
          </h2>
          <button class="btn btn-secondary btn-sm" @click="closeModal">{{ t('common.cancel') }}</button>
        </div>

        <form class="space-y-4" @submit.prevent="saveAccount">
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <label class="block">
              <span class="mb-1 block text-sm text-gray-600 dark:text-dark-300">{{ t('admin.users.email') }}</span>
              <input v-model="form.email" type="email" class="input" required />
            </label>
            <label class="block">
              <span class="mb-1 block text-sm text-gray-600 dark:text-dark-300">{{ t('admin.users.username') }}</span>
              <input v-model="form.username" type="text" class="input" />
            </label>
            <label class="block">
              <span class="mb-1 block text-sm text-gray-600 dark:text-dark-300">{{ t('admin.users.password') }}</span>
              <input v-model="form.password" type="password" class="input" :required="!editing" minlength="6" />
            </label>
            <label class="block">
              <span class="mb-1 block text-sm text-gray-600 dark:text-dark-300">{{ t('admin.adminAccounts.role') }}</span>
              <select v-model="form.role" class="input">
                <option value="operator">{{ t('admin.adminAccounts.operator') }}</option>
                <option value="admin">{{ t('admin.adminAccounts.admin') }}</option>
              </select>
            </label>
          </div>

          <label class="block">
            <span class="mb-1 block text-sm text-gray-600 dark:text-dark-300">{{ t('admin.users.notes') }}</span>
            <input v-model="form.notes" type="text" class="input" />
          </label>

          <div v-if="form.role === 'operator'">
            <div class="mb-2 text-sm font-medium text-gray-700 dark:text-dark-200">
              {{ t('admin.adminAccounts.allowedPages') }}
            </div>
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <label
                v-for="page in pageOptions"
                :key="page.key"
                class="flex items-center gap-2 rounded-md border border-gray-200 px-3 py-2 dark:border-dark-600"
              >
                <input v-model="form.operator_pages" type="checkbox" :value="page.key" />
                <span>{{ t(page.labelKey) }}</span>
              </label>
            </div>
          </div>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="btn btn-secondary" @click="closeModal">{{ t('common.cancel') }}</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? t('common.saving') : t('common.save') }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import adminAccountsAPI from '@/api/admin/adminAccounts'
import type { AdminUser } from '@/types'
import { ADMIN_PAGE_PERMISSIONS } from '@/utils/adminPermissions'

const { t } = useI18n()

const accounts = ref<AdminUser[]>([])
const loading = ref(false)
const saving = ref(false)
const showModal = ref(false)
const editing = ref<AdminUser | null>(null)

const filters = reactive({
  search: '',
  role: '' as '' | 'admin' | 'operator',
  status: '' as '' | 'active' | 'disabled',
  created_from: '',
  created_to: ''
})

const form = reactive({
  email: '',
  password: '',
  username: '',
  notes: '',
  role: 'operator' as 'admin' | 'operator',
  operator_pages: [] as string[]
})

const pageOptions = computed(() => ADMIN_PAGE_PERMISSIONS)

async function loadAccounts() {
  loading.value = true
  try {
    const result = await adminAccountsAPI.list(1, 100, {
      search: filters.search,
      role: filters.role,
      status: filters.status,
      created_from: filters.created_from,
      created_to: filters.created_to,
      sort_by: 'created_at',
      sort_order: 'desc'
    })
    accounts.value = result.items
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, {
    email: '',
    password: '',
    username: '',
    notes: '',
    role: 'operator',
    operator_pages: []
  })
  showModal.value = true
}

function openEdit(account: AdminUser) {
  editing.value = account
  Object.assign(form, {
    email: account.email,
    password: '',
    username: account.username || '',
    notes: account.notes || '',
    role: account.role === 'admin' ? 'admin' : 'operator',
    operator_pages: [...(account.operator_pages ?? [])]
  })
  showModal.value = true
}

function closeModal() {
  showModal.value = false
}

async function saveAccount() {
  saving.value = true
  try {
    const payload = {
      email: form.email,
      username: form.username,
      notes: form.notes,
      role: form.role,
      operator_pages: form.role === 'operator' ? form.operator_pages : []
    }
    if (editing.value) {
      await adminAccountsAPI.update(editing.value.id, {
        ...payload,
        password: form.password || undefined
      })
    } else {
      await adminAccountsAPI.create({
        ...payload,
        password: form.password
      })
    }
    closeModal()
    await loadAccounts()
  } finally {
    saving.value = false
  }
}

async function deleteAccount(account: AdminUser) {
  if (account.role === 'admin') return
  if (!window.confirm(t('admin.adminAccounts.confirmDelete'))) return
  await adminAccountsAPI.delete(account.id)
  await loadAccounts()
}

function roleLabel(role: string) {
  if (role === 'admin') return t('admin.adminAccounts.admin')
  if (role === 'operator') return t('admin.adminAccounts.operator')
  return role
}

function pageSummary(pages?: string[]) {
  if (!pages?.length) return t('admin.adminAccounts.noPages')
  const labels = pages
    .map((key) => pageOptions.value.find((page) => page.key === key))
    .filter(Boolean)
    .map((page) => t(page!.labelKey))
  return labels.join(', ')
}

function formatDate(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(loadAccounts)
</script>
