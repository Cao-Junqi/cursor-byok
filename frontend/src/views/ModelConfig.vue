<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ModelAdapterTestCard from "@/components/ModelAdapterTestCard.vue";
import { showModal } from "@/composables/useModal";
import {
  appState,
  createEmptyModelAdapter,
  deleteModelAdapterAt,
  duplicateModelAdapterAt,
  getModelAdapterTestResultByID,
  openModelEditorWindow,
  reloadUserConfig,
  runModelAdapterTest,
  saveModelAdapterAt,
  startModelAdapterTest,
  toUserError,
  updateProviderGroup,
} from "@/state/appState";
import { fetchModelList } from "@/services/clientApi";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

const BATCH_TEST_CONCURRENCY = 10;

const typeTabs = [
  { label: "OpenAI", value: "openai", icon: "icon-[bxl--openai]" },
  { label: "Anthropic", value: "anthropic", icon: "icon-[logos--claude-icon]" },
];

const activeType = ref("openai");
const batchTesting = ref(false);
const batchStopping = ref(false);
const batchTotal = ref(0);
const batchCompleted = ref(0);
const batchActiveCalls = new Set();
let batchStopRequested = false;

const filteredAdapters = computed(() =>
  appState.modelAdapters.filter((adapter) => adapter.type === activeType.value),
);

function groupKeyOf(adapter) {
  return `${adapter.type}::${adapter.baseURL}::${adapter.apiKey}`;
}

const groupedAdapters = computed(() => {
  const groups = new Map();
  filteredAdapters.value.forEach((adapter) => {
    const key = groupKeyOf(adapter);
    if (!groups.has(key)) {
      groups.set(key, { key, type: adapter.type, baseURL: adapter.baseURL, apiKey: adapter.apiKey, adapters: [] });
    }
    groups.get(key).adapters.push(adapter);
  });
  return Array.from(groups.values());
});

// 分组默认展开，这里记录被手动折叠的分组 key。
const collapsedGroups = ref(new Set());
function isGroupExpanded(key) {
  return !collapsedGroups.value.has(key);
}
function toggleGroup(key) {
  const next = new Set(collapsedGroups.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  collapsedGroups.value = next;
}

const batchButtonText = computed(() => {
  if (batchStopping.value) {
    return "停止中...";
  }
  if (!batchTesting.value) {
    return "测试全部";
  }
  return `停止测试 ${batchCompleted.value}/${batchTotal.value}`;
});

watch(
  () => appState.modelAdapters,
  (adapters) => {
    if (adapters.some((adapter) => adapter.type === activeType.value)) {
      return;
    }
    const fallback = typeTabs.find((tab) => adapters.some((adapter) => adapter.type === tab.value));
    activeType.value = fallback?.value ?? "openai";
  },
  { deep: true, immediate: true },
);

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "服务错误").trim() || "服务错误",
  });
}

function maskSecret(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  if (text.length <= 8) {
    return `${"*".repeat(Math.max(text.length - 2, 0))}${text.slice(-2)}`;
  }
  return `${text.slice(0, 4)}****${text.slice(-4)}`;
}

function typeLabel(type) {
  return type === "anthropic" ? "Anthropic" : "OpenAI";
}

function formatHost(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  try {
    const parsed = new URL(text);
    return parsed.host || text;
  } catch {
    return text.replace(/^https?:\/\//, "");
  }
}

async function openEditor(index = -1) {
  const adapter = index >= 0
    ? appState.modelAdapters[index]
    : {
        ...createEmptyModelAdapter(),
        type: activeType.value,
      };
  try {
    await openModelEditorWindow(index, adapter);
  } catch (error) {
    await showActionError("打开失败", toUserError(error));
  }
}

async function handleDeleteModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("删除失败", "模型配置不存在，无法删除");
    return;
  }
  const result = await deleteModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("删除失败", result.error);
  }
}

async function handleDuplicateModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("复制失败", "模型配置不存在，无法复制");
    return;
  }
  const result = await duplicateModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("复制失败", result.error);
  }
}

function getAdapterTestResult(adapter) {
  return getModelAdapterTestResultByID(adapter?.id);
}

function isAdapterTesting(adapter) {
  return getAdapterTestResult(adapter)?.status === "running";
}

async function handleTestModelAdapter(adapter) {
  try {
    await runModelAdapterTest(adapter);
  } catch (_error) {
    // 失败结果会通过事件同步到界面，这里不再额外弹窗打断用户。
  }
}

function isCancelError(error) {
  return String(error?.name || "").trim() === "CancelError";
}

async function stopBatchTesting() {
  if (!batchTesting.value || batchStopping.value) {
    return;
  }
  batchStopRequested = true;
  batchStopping.value = true;
  const activeCalls = Array.from(batchActiveCalls);
  await Promise.allSettled(
    activeCalls.map((call) => (typeof call?.cancel === "function" ? call.cancel("batch-stop") : undefined)),
  );
}

async function handleTestAllModelAdapters() {
  if (batchTesting.value) {
    await stopBatchTesting();
    return;
  }
  const adapters = filteredAdapters.value.slice();
  if (adapters.length === 0) {
    return;
  }
  batchStopRequested = false;
  batchTesting.value = true;
  batchStopping.value = false;
  batchTotal.value = adapters.length;
  batchCompleted.value = 0;
  let nextIndex = 0;
  try {
    const workers = Array.from({ length: Math.min(BATCH_TEST_CONCURRENCY, adapters.length) }, async () => {
      while (!batchStopRequested) {
        const currentIndex = nextIndex;
        nextIndex += 1;
        if (currentIndex >= adapters.length) {
          return;
        }
        const adapter = adapters[currentIndex];
        const call = startModelAdapterTest(adapter);
        batchActiveCalls.add(call);
        try {
          await call;
        } catch (error) {
          if (!isCancelError(error) && !batchStopRequested) {
            // 单个失败结果由卡片自行展示，这里继续后续测试。
          }
        } finally {
          batchActiveCalls.delete(call);
          batchCompleted.value += 1;
        }
      }
    });
    await Promise.allSettled(workers);
  } finally {
    batchActiveCalls.clear();
    batchStopRequested = false;
    batchTesting.value = false;
    batchStopping.value = false;
  }
}

// ── 供应商分组操作 ────────────────────────────────────────────────────────────
const groupEditModal = ref(false);
const groupEditTarget = reactive({ type: "openai", baseURL: "", apiKey: "" }); // 原始身份，用于定位分组
const groupEditForm = reactive({ baseURL: "", apiKey: "" });
const groupEditSaving = ref(false);
const groupEditModelCount = ref(0);

function openGroupEditModal(group) {
  groupEditTarget.type = group.type;
  groupEditTarget.baseURL = group.baseURL;
  groupEditTarget.apiKey = group.apiKey;
  groupEditForm.baseURL = group.baseURL;
  groupEditForm.apiKey = group.apiKey;
  groupEditModelCount.value = group.adapters.length;
  groupEditModal.value = true;
}

function closeGroupEditModal() {
  groupEditModal.value = false;
}

async function handleGroupEditSubmit() {
  groupEditSaving.value = true;
  try {
    const result = await updateProviderGroup(groupEditTarget.type, groupEditTarget.baseURL, groupEditTarget.apiKey, {
      baseURL: groupEditForm.baseURL.trim(),
      apiKey: groupEditForm.apiKey.trim(),
    });
    if (!result.ok) {
      await showActionError("保存失败", result.error);
      return;
    }
    closeGroupEditModal();
  } finally {
    groupEditSaving.value = false;
  }
}

const addModelModal = ref(false);
const addModelTarget = reactive({ type: "openai", baseURL: "", apiKey: "" });
const addModelForm = reactive({ modelID: "", displayName: "" });
const addModelSaving = ref(false);

function openAddModelModal(group) {
  addModelTarget.type = group.type;
  addModelTarget.baseURL = group.baseURL;
  addModelTarget.apiKey = group.apiKey;
  addModelForm.modelID = "";
  addModelForm.displayName = "";
  addModelModal.value = true;
}

function closeAddModelModal() {
  addModelModal.value = false;
}

async function handleAddModelSubmit() {
  const modelID = addModelForm.modelID.trim();
  if (!modelID) return;
  addModelSaving.value = true;
  try {
    const adapter = {
      ...createEmptyModelAdapter(),
      type: addModelTarget.type,
      baseURL: addModelTarget.baseURL,
      apiKey: addModelTarget.apiKey,
      modelID,
      displayName: addModelForm.displayName.trim() || modelID,
    };
    const result = await saveModelAdapterAt(-1, adapter);
    if (!result.ok) {
      await showActionError("添加失败", result.error);
      return;
    }
    closeAddModelModal();
  } finally {
    addModelSaving.value = false;
  }
}
// ─────────────────────────────────────────────────────────────────────────────

// ── 批量导入 ──────────────────────────────────────────────────────────────────
const ANTHROPIC_OFFICIAL_BASE_URL = "https://api.anthropic.com";
const importModal = ref(false);
const importForm = reactive({ type: "openai", baseURL: "", apiKey: "", providerName: "" });
const importFetching = ref(false);
const importModels = ref([]);   // [{id, displayName}]
const importSelected = ref(new Set());
const importNames = reactive({}); // { [model.id]: 用户编辑后的展示名 }
const importError = ref("");
const importAdding = ref(false);

const importAllSelected = computed(
  () => importModels.value.length > 0 && importSelected.value.size === importModels.value.length,
);

function defaultImportName(model) {
  const raw = model.displayName || model.id;
  return importForm.providerName.trim() ? `${importForm.providerName.trim()} ${raw}` : raw;
}

function importNameFor(model) {
  return importNames[model.id] ?? defaultImportName(model);
}

function setImportType(type) {
  importForm.type = type;
  importModels.value = [];
  importError.value = "";
  if (type === "anthropic" && !importForm.baseURL.trim()) {
    importForm.baseURL = ANTHROPIC_OFFICIAL_BASE_URL;
  }
}

function openImportModal() {
  importModal.value = true;
  importModels.value = [];
  importSelected.value = new Set();
  for (const key of Object.keys(importNames)) delete importNames[key];
  importError.value = "";
  importFetching.value = false;
  importAdding.value = false;
}

function closeImportModal() {
  importModal.value = false;
}

async function handleFetchModels() {
  importError.value = "";
  importModels.value = [];
  importSelected.value = new Set();
  for (const key of Object.keys(importNames)) delete importNames[key];
  importFetching.value = true;
  try {
    const result = await fetchModelList({
      type: importForm.type,
      baseURL: importForm.baseURL.trim(),
      apiKey: importForm.apiKey.trim(),
    });
    if (result.error) {
      importError.value = result.error;
    } else {
      importModels.value = result.models || [];
      // 默认全选
      importSelected.value = new Set(importModels.value.map((m) => m.id));
    }
  } catch (err) {
    importError.value = toUserError(err);
  } finally {
    importFetching.value = false;
  }
}

function toggleImportModel(id) {
  const next = new Set(importSelected.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  importSelected.value = next;
}

function toggleImportAll() {
  if (importAllSelected.value) {
    importSelected.value = new Set();
  } else {
    importSelected.value = new Set(importModels.value.map((m) => m.id));
  }
}

async function handleBatchImport() {
  const toAdd = importModels.value.filter((m) => importSelected.value.has(m.id));
  if (toAdd.length === 0) return;
  importAdding.value = true;
  try {
    for (const model of toAdd) {
      const adapter = {
        ...createEmptyModelAdapter(),
        type: importForm.type,
        baseURL: importForm.baseURL.trim(),
        apiKey: importForm.apiKey.trim(),
        modelID: model.id,
        displayName: importNameFor(model).trim() || defaultImportName(model),
        tooltipData: model.displayName || model.id,
      };
      const result = await saveModelAdapterAt(-1, adapter);
      if (!result.ok) {
        await showModal({ title: "添加失败", content: result.error });
        break;
      }
    }
    closeImportModal();
  } catch (err) {
    await showModal({ title: "添加失败", content: toUserError(err) });
  } finally {
    importAdding.value = false;
  }
}
// ─────────────────────────────────────────────────────────────────────────────

onMounted(async () => {
  await reloadUserConfig({ modelAdaptersOnly: true }).catch(() => { });
});

onBeforeUnmount(() => {
  void stopBatchTesting();
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col p-4 pt-0 text-ink-primary overflow-hidden">
    <div class="shrink-0 pb-4">
      <div class="flex items-center justify-between gap-4">
        <div class="center-row gap-2">
          <button
            v-for="tab in typeTabs"
            :key="tab.value"
            type="button"
            class="center-row gap-2 rounded-[8px] border px-3 py-2 text-sm transition-colors duration-150"
            :class="activeType === tab.value
              ? 'border-brand-500 bg-brand-500/15 text-white'
              : 'border-line bg-surface-hover text-ink-secondary hover:border-line hover:text-ink-primary'"
            @click="activeType = tab.value"
          >
            <span :class="[tab.icon, 'text-[16px]']"></span>
            <span>{{ tab.label }}</span>
          </button>
        </div>
        <div class="center-row gap-2">
          <Button
            variant="default"
            :disabled="appState.configSaving || (!batchTesting && filteredAdapters.length === 0)"
            @click="handleTestAllModelAdapters"
          >
            {{ batchButtonText }}
          </Button>
          <Button variant="default" :disabled="appState.configSaving || batchTesting" @click="openImportModal">批量导入</Button>
          <Button variant="primary" :disabled="appState.configSaving || batchTesting" @click="openEditor()">新增模型</Button>
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1">
      <div v-if="filteredAdapters.length === 0"
        class="flex h-full min-h-[220px] items-center justify-center rounded-[8px] border border-dashed border-line-strong bg-surface-input px-4 text-sm text-ink-secondary">
        当前还没有配置任何 {{ typeLabel(activeType) }} 模型。
      </div>

      <div v-else class="h-full min-h-0 overflow-y-auto pr-1 flex flex-col gap-3 pb-1">
        <Card v-for="group in groupedAdapters" :key="group.key">
          <div class="flex flex-col gap-3">
            <div class="flex items-start justify-between gap-3">
              <button
                type="button"
                class="flex min-w-0 flex-1 items-start gap-2 text-left"
                @click="toggleGroup(group.key)"
              >
                <span
                  :class="[isGroupExpanded(group.key) ? 'icon-[mdi--chevron-down]' : 'icon-[mdi--chevron-right]', 'mt-1 shrink-0 text-[18px] text-ink-muted']"
                ></span>
                <span
                  class="center-row shrink-0 gap-1 rounded-[999px] border border-line-strong px-[7px] py-[4px] text-[11px] font-medium text-ink-secondary"
                >
                  <span class="icon-[bxl--openai] text-[14px] !text-white" v-if="group.type === 'openai'"></span>
                  <span class="icon-[logos--claude-icon] text-[14px]" v-else></span>
                  <span>{{ typeLabel(group.type) }}</span>
                </span>
                <div class="min-w-0 flex-1">
                  <div class="truncate text-base font-medium text-white" :title="group.baseURL">{{ formatHost(group.baseURL) }}</div>
                  <div class="mt-0.5 truncate text-xs text-ink-muted">{{ maskSecret(group.apiKey) }} · {{ group.adapters.length }} 个模型</div>
                </div>
              </button>
              <div class="center-row shrink-0 gap-2">
                <Button variant="default" :disabled="appState.configSaving" @click="openAddModelModal(group)">添加模型</Button>
                <Button variant="default" :disabled="appState.configSaving" @click="openGroupEditModal(group)">编辑供应商</Button>
              </div>
            </div>

            <div v-if="isGroupExpanded(group.key)" class="flex flex-col gap-2 border-t border-line pt-3">
              <div
                v-for="adapter in group.adapters"
                :key="adapter.id || `${adapter.baseURL}-${adapter.modelID}`"
                class="flex flex-col gap-2 rounded-[8px] bg-surface-input px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between"
              >
                <div class="min-w-0 flex-1">
                  <div class="truncate text-sm font-medium text-white">{{ adapter.displayName }}</div>
                  <div class="mt-0.5 truncate text-xs text-ink-muted">{{ adapter.modelID }}</div>
                </div>
                <div class="center-row shrink-0 flex-wrap gap-2">
                  <ModelAdapterTestCard
                    compact
                    title="测试"
                    empty-text="未测试"
                    :result="getAdapterTestResult(adapter)"
                  />
                  <Button
                    variant="default"
                    :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)"
                    @click="handleTestModelAdapter(adapter)"
                  >
                    {{ isAdapterTesting(adapter) ? "测试中..." : "测试" }}
                  </Button>
                  <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">编辑</Button>
                  <Button variant="default" :disabled="appState.configSaving" @click="handleDuplicateModelAdapter(appState.modelAdapters.indexOf(adapter))">复制</Button>
                  <Button variant="text" :disabled="appState.configSaving"
                    @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">删除</Button>
                </div>
              </div>
            </div>
          </div>
        </Card>
      </div>
    </div>
  </div>

  <!-- 编辑供应商模态框 -->
  <Teleport to="body">
    <Transition name="modal-mask">
      <div
        v-if="groupEditModal"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
        @click.self="closeGroupEditModal"
      >
        <div class="w-full max-w-sm rounded-[10px] border border-line bg-surface-input flex flex-col">
          <div class="flex items-center justify-between px-5 py-4 border-b border-line">
            <h2 class="text-base font-medium text-white">编辑供应商</h2>
            <button class="text-ink-muted hover:text-white transition-colors" @click="closeGroupEditModal">
              <span class="icon-[mdi--close] text-[18px]"></span>
            </button>
          </div>
          <div class="px-5 py-4 flex flex-col gap-3">
            <p class="text-xs text-ink-muted">修改将同步应用到该供应商下的全部 {{ groupEditModelCount }} 个模型。</p>
            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">接口地址</label>
              <input
                v-model="groupEditForm.baseURL"
                type="text"
                :placeholder="groupEditTarget.type === 'anthropic' ? 'https://api.anthropic.com' : 'https://api.openai.com'"
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">访问密钥</label>
              <input
                v-model="groupEditForm.apiKey"
                type="password"
                placeholder="sk-..."
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>
          </div>
          <div class="flex justify-end gap-2 px-5 py-4 border-t border-line">
            <Button variant="default" :disabled="groupEditSaving" @click="closeGroupEditModal">取消</Button>
            <Button
              variant="primary"
              :disabled="groupEditSaving || !groupEditForm.baseURL.trim() || !groupEditForm.apiKey.trim()"
              @click="handleGroupEditSubmit"
            >
              {{ groupEditSaving ? "保存中..." : "保存" }}
            </Button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 添加模型模态框 -->
  <Teleport to="body">
    <Transition name="modal-mask">
      <div
        v-if="addModelModal"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
        @click.self="closeAddModelModal"
      >
        <div class="w-full max-w-sm rounded-[10px] border border-line bg-surface-input flex flex-col">
          <div class="flex items-center justify-between px-5 py-4 border-b border-line">
            <h2 class="text-base font-medium text-white">添加模型</h2>
            <button class="text-ink-muted hover:text-white transition-colors" @click="closeAddModelModal">
              <span class="icon-[mdi--close] text-[18px]"></span>
            </button>
          </div>
          <div class="px-5 py-4 flex flex-col gap-3">
            <p class="text-xs text-ink-muted">接入点与密钥继承自 {{ formatHost(addModelTarget.baseURL) }}。</p>
            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">模型 ID</label>
              <input
                v-model="addModelForm.modelID"
                type="text"
                placeholder="例如：gpt-4.1 / claude-sonnet-4-5"
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">展示名称（可选）</label>
              <input
                v-model="addModelForm.displayName"
                type="text"
                placeholder="留空则使用模型 ID"
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>
          </div>
          <div class="flex justify-end gap-2 px-5 py-4 border-t border-line">
            <Button variant="default" :disabled="addModelSaving" @click="closeAddModelModal">取消</Button>
            <Button variant="primary" :disabled="addModelSaving || !addModelForm.modelID.trim()" @click="handleAddModelSubmit">
              {{ addModelSaving ? "添加中..." : "添加" }}
            </Button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 批量导入模态框 -->
  <Teleport to="body">
    <Transition name="modal-mask">
      <div
        v-if="importModal"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
        @click.self="closeImportModal"
      >
        <div class="w-full max-w-md rounded-[10px] border border-line bg-surface-input flex flex-col max-h-[85vh]">
          <!-- 头部 -->
          <div class="flex items-center justify-between px-5 py-4 border-b border-line shrink-0">
            <h2 class="text-base font-medium text-white">批量导入模型</h2>
            <button
              class="text-ink-muted hover:text-white transition-colors"
              @click="closeImportModal"
            >
              <span class="icon-[mdi--close] text-[18px]"></span>
            </button>
          </div>

          <!-- 表单区 -->
          <div class="px-5 py-4 flex flex-col gap-3 shrink-0">
            <!-- 类型选项卡 -->
            <div class="center-row gap-2">
              <button
                v-for="tab in typeTabs"
                :key="tab.value"
                type="button"
                class="center-row gap-1.5 rounded-[6px] border px-3 py-1.5 text-sm transition-colors"
                :class="importForm.type === tab.value
                  ? 'border-brand-500 bg-brand-500/15 text-white'
                  : 'border-line bg-surface-hover text-ink-secondary hover:border-line hover:text-ink-primary'"
                @click="setImportType(tab.value)"
              >
                <span :class="[tab.icon, 'text-[14px]']"></span>
                {{ tab.label }}
              </button>
            </div>

            <!-- base URL -->
            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">接口地址</label>
              <input
                v-model="importForm.baseURL"
                type="text"
                :placeholder="importForm.type === 'anthropic' ? 'https://api.anthropic.com' : 'https://api.openai.com'"
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>

            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">供应商名称（可选，作为模型名前缀）</label>
              <input
                v-model="importForm.providerName"
                type="text"
                placeholder="如：硅基、DeepSeek、OpenRouter..."
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>

            <div class="flex flex-col gap-1">
              <label class="text-xs text-ink-muted">访问密钥</label>
              <input
                v-model="importForm.apiKey"
                type="password"
                placeholder="sk-..."
                class="w-full rounded-[6px] border border-line bg-surface-input px-3 py-2 text-sm text-white placeholder-[#555] outline-none focus:border-[#555]"
              />
            </div>

            <Button
              variant="default"
              :disabled="importFetching || (!importForm.apiKey.trim())"
              class="w-full justify-center"
              @click="handleFetchModels"
            >
              <span v-if="importFetching" class="icon-[mdi--loading] animate-spin text-[16px]"></span>
              <span v-else class="icon-[mdi--cloud-download-outline] text-[16px]"></span>
              {{ importFetching ? "获取中..." : "获取模型列表" }}
            </Button>

            <div v-if="importError" class="rounded-[6px] border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">
              {{ importError }}
            </div>
          </div>

          <!-- 模型列表 -->
          <div v-if="importModels.length > 0" class="flex flex-col min-h-0 flex-1 border-t border-line">
            <div class="flex items-center justify-between px-5 py-2 shrink-0">
              <span class="text-xs text-ink-muted">共 {{ importModels.length }} 个模型，已选 {{ importSelected.size }}</span>
              <button
                class="text-xs text-brand-500 hover:text-brand-400 transition-colors"
                @click="toggleImportAll"
              >
                {{ importAllSelected ? "取消全选" : "全选" }}
              </button>
            </div>
            <div class="overflow-y-auto px-5 pb-2 flex flex-col gap-1">
              <label
                v-for="model in importModels"
                :key="model.id"
                class="flex items-center gap-3 rounded-[6px] px-2 py-1.5 cursor-pointer hover:bg-surface-hover transition-colors"
              >
                <input
                  type="checkbox"
                  :checked="importSelected.has(model.id)"
                  class="accent-brand-500 w-4 h-4 shrink-0"
                  @change="toggleImportModel(model.id)"
                />
                <div class="min-w-0 flex-1 flex flex-col gap-0.5">
                  <input
                    :value="importNameFor(model)"
                    type="text"
                    class="w-full bg-transparent text-sm text-white truncate outline-none border-b border-transparent hover:border-line-strong focus:border-[#555] transition-colors"
                    @click.stop
                    @input="importNames[model.id] = $event.target.value"
                  />
                  <div v-if="model.displayName !== model.id" class="text-xs text-ink-muted truncate">{{ model.id }}</div>
                </div>
              </label>
            </div>
          </div>

          <!-- 底部操作 -->
          <div v-if="importModels.length > 0" class="px-5 py-4 border-t border-line center-row justify-end gap-2 shrink-0">
            <Button variant="default" :disabled="importAdding" @click="closeImportModal">取消</Button>
            <Button
              variant="primary"
              :disabled="importAdding || importSelected.size === 0"
              @click="handleBatchImport"
            >
              <span v-if="importAdding" class="icon-[mdi--loading] animate-spin text-[16px]"></span>
              {{ importAdding ? "添加中..." : `添加选中 (${importSelected.size})` }}
            </Button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
