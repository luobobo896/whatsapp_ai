import fs from "node:fs";
import path from "node:path";
import { rootDir } from "./config.js";

const KNOWLEDGE_CONFIG_PATH = path.join(rootDir, "config", "knowledge.json");

export function readKnowledgeConfig() {
  return JSON.parse(fs.readFileSync(KNOWLEDGE_CONFIG_PATH, "utf8"));
}

export function getRoleIds() {
  const knowledge = readKnowledgeConfig();
  return new Set((knowledge.roles || []).map((role) => role.id));
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {string}
 */
function requireText(value, field) {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error(`${field} must be a non-empty string`);
  }
  return value.trim();
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {string[]}
 */
function requireTextArray(value, field) {
  if (!Array.isArray(value) || value.length === 0) {
    throw new Error(`${field} must be a non-empty string array`);
  }
  return value.map((item, index) => requireText(item, `${field}[${index}]`));
}

/**
 * @param {unknown} value
 * @param {string} field
 * @returns {string[]}
 */
function optionalTextArray(value, field) {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be a string array`);
  }
  return value.map((item, index) => requireText(item, `${field}[${index}]`));
}

/**
 * @param {unknown} knowledge
 * @returns {{roles: Array<Record<string, unknown>>}}
 */
export function normalizeKnowledgeConfig(knowledge) {
  if (!knowledge || typeof knowledge !== "object" || !Array.isArray(knowledge.roles) || knowledge.roles.length === 0) {
    throw new Error("knowledge.roles must be a non-empty array");
  }
  const roleIds = new Set();
  const normalizedRoles = knowledge.roles.map((role, roleIndex) => {
    if (!role || typeof role !== "object") {
      throw new Error(`roles[${roleIndex}] must be an object`);
    }
    const id = requireText(role.id, `roles[${roleIndex}].id`);
    if (roleIds.has(id)) {
      throw new Error(`duplicate role id: ${id}`);
    }
    roleIds.add(id);
    if (!Array.isArray(role.products) || role.products.length === 0) {
      throw new Error(`roles[${roleIndex}].products must be a non-empty array`);
    }
    const products = role.products.map((product, productIndex) => {
      if (!product || typeof product !== "object") {
        throw new Error(`roles[${roleIndex}].products[${productIndex}] must be an object`);
      }
      return {
        id: requireText(product.id, `roles[${roleIndex}].products[${productIndex}].id`),
        name: requireText(product.name, `roles[${roleIndex}].products[${productIndex}].name`),
        aliases: optionalTextArray(product.aliases, `roles[${roleIndex}].products[${productIndex}].aliases`),
        price: requireText(product.price, `roles[${roleIndex}].products[${productIndex}].price`),
        sizes: optionalTextArray(product.sizes, `roles[${roleIndex}].products[${productIndex}].sizes`),
        colors: optionalTextArray(product.colors, `roles[${roleIndex}].products[${productIndex}].colors`),
        delivery: requireText(product.delivery, `roles[${roleIndex}].products[${productIndex}].delivery`),
        description: requireText(product.description, `roles[${roleIndex}].products[${productIndex}].description`),
        sellingPoints: optionalTextArray(product.sellingPoints, `roles[${roleIndex}].products[${productIndex}].sellingPoints`),
        notes: optionalTextArray(product.notes, `roles[${roleIndex}].products[${productIndex}].notes`)
      };
    });
    const faq = Array.isArray(role.faq) ? role.faq.map((item, faqIndex) => ({
      question: requireText(item?.question, `roles[${roleIndex}].faq[${faqIndex}].question`),
      answer: requireText(item?.answer, `roles[${roleIndex}].faq[${faqIndex}].answer`)
    })) : [];
    return {
      id,
      name: requireText(role.name, `roles[${roleIndex}].name`),
      description: requireText(role.description, `roles[${roleIndex}].description`),
      keywords: requireTextArray(role.keywords, `roles[${roleIndex}].keywords`),
      unknownReply: requireText(role.unknownReply, `roles[${roleIndex}].unknownReply`),
      rules: requireTextArray(role.rules, `roles[${roleIndex}].rules`),
      products,
      faq
    };
  });
  return { roles: normalizedRoles };
}

/**
 * @param {string[]} roles
 * @returns {string[]}
 */
export function validateRoleIds(roles) {
  const roleIds = getRoleIds();
  const selected = Array.isArray(roles) ? roles.map((role) => String(role).trim()).filter(Boolean) : [];
  for (const role of selected) {
    if (!roleIds.has(role)) {
      throw new Error(`unknown role: ${role}`);
    }
  }
  return selected;
}
