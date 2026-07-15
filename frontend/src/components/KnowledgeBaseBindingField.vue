<script setup>
import { computed } from "vue";
import { CheckCheck, Trash2 } from "lucide-vue-next";

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  knowledgeBases: { type: Array, default: () => [] },
  loading: Boolean,
  disabled: Boolean,
});
const emit = defineEmits(["update:modelValue"]);

const options = computed(() => props.knowledgeBases.map((base) => ({
  value: base.id,
  label: base.name,
})));
const selectedCount = computed(() => props.modelValue.length);

function selectAll() {
  emit("update:modelValue", options.value.map((option) => option.value));
}

function clearSelection() {
  emit("update:modelValue", []);
}
</script>

<template>
  <div class="knowledge-binding-field">
    <div class="knowledge-binding-heading">
      <div>
        <label class="knowledge-binding-label">绑定知识库</label>
        <p class="knowledge-binding-help">支持搜索与多选，大量知识库会以虚拟列表加载。</p>
      </div>
      <div class="knowledge-binding-actions">
        <el-button
          text
          size="small"
          :icon="CheckCheck"
          :disabled="disabled || !options.length || selectedCount === options.length"
          @click="selectAll"
        >
          全选
        </el-button>
        <el-button
          text
          size="small"
          :icon="Trash2"
          :disabled="disabled || !selectedCount"
          @click="clearSelection"
        >
          清空
        </el-button>
      </div>
    </div>
    <el-select-v2
      :model-value="modelValue"
      :options="options"
      :loading="loading"
      :disabled="disabled"
      multiple
      filterable
      clearable
      collapse-tags
      :max-collapse-tags="1"
      :height="274"
      :item-height="36"
      placeholder="搜索并选择知识库"
      no-match-text="未找到匹配的知识库"
      no-data-text="暂无可用知识库"
      popper-class="knowledge-binding-popper"
      aria-label="绑定知识库"
      class="knowledge-binding-select"
      @update:model-value="emit('update:modelValue', $event)"
    />
    <div class="knowledge-binding-summary" aria-live="polite">
      <span>{{ selectedCount ? `已选择 ${selectedCount} 个知识库` : "尚未绑定知识库" }}</span>
      <span v-if="knowledgeBases.length">共 {{ knowledgeBases.length }} 个可用</span>
    </div>
  </div>
</template>
