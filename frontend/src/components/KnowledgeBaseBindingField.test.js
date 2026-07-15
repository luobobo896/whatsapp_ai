import { describe, expect, it } from "vitest";
import { shallowMount } from "@vue/test-utils";
import KnowledgeBaseBindingField from "./KnowledgeBaseBindingField.vue";

function mountField(props) {
  return shallowMount(KnowledgeBaseBindingField, {
    props,
    global: {
      stubs: {
        ElButton: { template: "<button @click=\"$emit('click')\"><slot /></button>" },
        ElSelectV2: { template: "<div><slot /></div>" },
      },
    },
  });
}

describe("KnowledgeBaseBindingField", () => {
  it("selects every available knowledge base without rendering every selected tag", async () => {
    const knowledgeBases = Array.from({ length: 1000 }, (_, index) => ({
      id: `base-${index}`,
      name: `知识库 ${index}`,
    }));
    const wrapper = mountField({ modelValue: [], knowledgeBases });

    await wrapper.get("button").trigger("click");

    expect(wrapper.emitted("update:modelValue")[0][0]).toHaveLength(1000);
    expect(wrapper.text()).toContain("共 1000 个可用");
  });

  it("clears an existing binding set", async () => {
    const wrapper = mountField({
      modelValue: ["base-1", "base-2"],
      knowledgeBases: [{ id: "base-1", name: "商品知识库" }, { id: "base-2", name: "售后知识库" }],
    });

    await wrapper.findAll("button")[1].trigger("click");

    expect(wrapper.emitted("update:modelValue")[0][0]).toEqual([]);
  });
});
