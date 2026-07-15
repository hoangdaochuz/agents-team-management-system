import { request } from "./client";
import type { Provider, ProviderKey } from "./types";

export function listProviderKeys() {
  return request<ProviderKey[]>("/provider-keys");
}

export function setProviderKey(provider: Provider, apiKey: string) {
  return request<ProviderKey>("/provider-keys", {
    method: "POST",
    body: JSON.stringify({ provider, api_key: apiKey }),
  });
}

export function updateProviderKey(provider: Provider, apiKey: string) {
  return request<ProviderKey>(`/provider-keys/${provider}`, {
    method: "PUT",
    body: JSON.stringify({ api_key: apiKey }),
  });
}

export function deleteProviderKey(provider: Provider) {
  return request<void>(`/provider-keys/${provider}`, { method: "DELETE" });
}

export const providerKeys = {
  listProviderKeys,
  setProviderKey,
  updateProviderKey,
  deleteProviderKey,
};
