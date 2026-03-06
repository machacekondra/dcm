const BASE = '/api/v1';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || `Request failed: ${res.status}`);
  return data as T;
}

// --- Types ---

export interface ApplicationRecord {
  name: string;
  labels: Record<string, string> | null;
  components: Component[];
  createdAt: string;
  updatedAt: string;
}

export interface Component {
  name: string;
  type: string;
  dependsOn?: string[];
  labels?: Record<string, string>;
  properties?: Record<string, unknown>;
}

export interface PolicyRecord {
  name: string;
  rules: PolicyRule[];
  createdAt: string;
  updatedAt: string;
}

export interface PolicyRule {
  name?: string;
  priority?: number;
  match: {
    type?: string;
    labels?: Record<string, string>;
    expression?: string;
  };
  providers: {
    required?: string;
    preferred?: string[];
    forbidden?: string[];
    strategy?: string;
  };
  properties?: Record<string, unknown>;
}

export interface DeploymentRecord {
  id: string;
  application: string;
  status: string;
  plan?: Plan;
  state?: DeploymentState;
  error?: string;
  policies: string[];
  createdAt: string;
  updatedAt: string;
}

export interface Plan {
  appName: string;
  steps: PlanStep[];
}

export interface PlanStep {
  component: string;
  diff: {
    action: string;
    resource: string;
    type: string;
    provider: string;
    before?: Record<string, unknown>;
    after?: Record<string, unknown>;
  };
  matchedRules?: string[];
}

export interface DeploymentState {
  version: number;
  app: string;
  resources: Record<string, Resource>;
  updatedAt: string;
}

export interface Resource {
  name: string;
  type: string;
  provider: string;
  properties?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  status: string;
}

export interface HistoryRecord {
  id: number;
  deploymentId: string;
  action: string;
  details?: unknown;
  createdAt: string;
}

export interface ProviderInfo {
  name: string;
  capabilities: string[];
}

export interface EvaluateResult {
  component: string;
  type: string;
  matchedRules: string[] | null;
  required?: string;
  preferred?: string[];
  forbidden?: string[];
  strategy?: string;
  properties?: Record<string, unknown>;
  selected?: string;
  error?: string;
}

// --- Applications ---

export const applications = {
  list: () => request<ApplicationRecord[]>('/applications'),
  get: (name: string) => request<ApplicationRecord>(`/applications/${name}`),
  create: (body: { name: string; labels?: Record<string, string>; components: Component[] }) =>
    request<ApplicationRecord>('/applications', { method: 'POST', body: JSON.stringify(body) }),
  update: (name: string, body: { labels?: Record<string, string>; components: Component[] }) =>
    request<ApplicationRecord>(`/applications/${name}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (name: string) => request<void>(`/applications/${name}`, { method: 'DELETE' }),
  validate: (name: string) =>
    request<{ valid: boolean; errors?: string[] }>(`/applications/${name}/validate`, { method: 'POST' }),
};

// --- Policies ---

export const policies = {
  list: () => request<PolicyRecord[]>('/policies'),
  get: (name: string) => request<PolicyRecord>(`/policies/${name}`),
  create: (body: { name: string; rules: PolicyRule[] }) =>
    request<PolicyRecord>('/policies', { method: 'POST', body: JSON.stringify(body) }),
  update: (name: string, body: { rules: PolicyRule[] }) =>
    request<PolicyRecord>(`/policies/${name}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (name: string) => request<void>(`/policies/${name}`, { method: 'DELETE' }),
  validate: (name: string) =>
    request<{ valid: boolean; errors?: string[] }>(`/policies/${name}/validate`, { method: 'POST' }),
  evaluate: (body: { application: string; policies: string[] }) =>
    request<EvaluateResult[]>('/policies/evaluate', { method: 'POST', body: JSON.stringify(body) }),
};

// --- Deployments ---

export const deployments = {
  list: () => request<DeploymentRecord[]>('/deployments'),
  get: (id: string) => request<DeploymentRecord>(`/deployments/${id}`),
  create: (body: { application: string; policies?: string[]; dryRun?: boolean }) =>
    request<DeploymentRecord>('/deployments', { method: 'POST', body: JSON.stringify(body) }),
  destroy: (id: string) => request<DeploymentRecord>(`/deployments/${id}`, { method: 'DELETE' }),
  plan: (id: string) => request<Plan>(`/deployments/${id}/plan`, { method: 'POST' }),
  history: (id: string) => request<HistoryRecord[]>(`/deployments/${id}/history`),
};

// --- Providers ---

export const providers = {
  list: () => request<ProviderInfo[]>('/providers'),
};
