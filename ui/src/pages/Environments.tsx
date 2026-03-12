import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  FormGroup,
  Label,
  LabelGroup,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  PageSection,
  Content,
  TextArea,
  TextInput,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Tooltip } from '@patternfly/react-core';
import { environments, type EnvironmentRecord, type HealthCheck } from '../api/client';

export default function Environments() {
  const [list, setList] = useState<EnvironmentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);
  const [editingEnv, setEditingEnv] = useState<EnvironmentRecord | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(() => {
    setLoading(true);
    environments.list().then(setList).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete environment "${name}"?`)) return;
    try {
      await environments.delete(name);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Environments</Content>
      </PageSection>
      <PageSection>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button onClick={() => setCreateOpen(true)}>Create environment</Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        {!loading && list.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No environments configured.</EmptyStateBody>
            <EmptyStateFooter>
              <EmptyStateActions>
                <Button onClick={() => setCreateOpen(true)}>Create environment</Button>
              </EmptyStateActions>
            </EmptyStateFooter>
          </EmptyState>
        ) : (
          <Table aria-label="Environments">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Provider</Th>
                <Th>Capabilities</Th>
                <Th>Labels</Th>
                <Th>Resources</Th>
                <Th>Cost</Th>
                <Th>Status</Th>
                <Th>Health</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {list.map(env => (
                <Tr key={env.name}>
                  <Td dataLabel="Name"><strong>{env.name}</strong></Td>
                  <Td dataLabel="Provider"><Label isCompact color="blue">{env.provider}</Label></Td>
                  <Td dataLabel="Capabilities">
                    {env.capabilities && env.capabilities.length > 0 ? (
                      <LabelGroup>
                        {env.capabilities.map(cap => (
                          <Label key={cap} isCompact color="purple">{cap}</Label>
                        ))}
                      </LabelGroup>
                    ) : '—'}
                  </Td>
                  <Td dataLabel="Labels">
                    {env.labels && Object.keys(env.labels).length > 0 ? (
                      <LabelGroup>
                        {Object.entries(env.labels).map(([k, v]) => (
                          <Label key={k} isCompact>{k}={v}</Label>
                        ))}
                      </LabelGroup>
                    ) : '—'}
                  </Td>
                  <Td dataLabel="Resources">
                    {env.resources ? `${env.resources.cpu}m CPU, ${env.resources.memory}MB, ${env.resources.pods} pods` : '—'}
                  </Td>
                  <Td dataLabel="Cost">
                    {env.cost ? `${env.cost.tier} ($${env.cost.hourlyRate}/hr)` : '—'}
                  </Td>
                  <Td dataLabel="Status"><Label isCompact color={env.status === 'active' ? 'green' : 'grey'}>{env.status}</Label></Td>
                  <Td dataLabel="Health">
                    <HealthLabel status={env.healthStatus} message={env.healthMessage} lastHeartbeat={env.lastHeartbeat} />
                  </Td>
                  <Td dataLabel="Actions" isActionCell>
                    <Button variant="secondary" size="sm" onClick={() => setEditingEnv(env)} style={{ marginRight: 8 }}>
                      Edit
                    </Button>
                    <Button variant="danger" size="sm" onClick={() => handleDelete(env.name)}>
                      Delete
                    </Button>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
        <CreateEnvironmentModal isOpen={isCreateOpen} onClose={() => setCreateOpen(false)} onCreated={load} />
        <EditEnvironmentModal env={editingEnv} onClose={() => setEditingEnv(null)} onSaved={load} />
      </PageSection>
    </>
  );
}

function CreateEnvironmentModal({ isOpen, onClose, onCreated }: { isOpen: boolean; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [provider, setProvider] = useState('');
  const [labelsStr, setLabelsStr] = useState('');
  const [capsStr, setCapsStr] = useState('');
  const [configStr, setConfigStr] = useState('{}');
  const [cpu, setCpu] = useState('');
  const [memory, setMemory] = useState('');
  const [pods, setPods] = useState('');
  const [costTier, setCostTier] = useState('');
  const [costRate, setCostRate] = useState('');
  const [hcUrl, setHcUrl] = useState('');
  const [hcInterval, setHcInterval] = useState('');
  const [hcTimeout, setHcTimeout] = useState('');
  const [hcInsecure, setHcInsecure] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async () => {
    setError('');
    setSubmitting(true);
    try {
      let labels: Record<string, string> | undefined;
      if (labelsStr.trim()) {
        labels = JSON.parse(labelsStr);
      }
      const capabilities = capsStr.trim() ? capsStr.split(',').map(s => s.trim()).filter(Boolean) : undefined;
      const config = JSON.parse(configStr);
      const resources = cpu || memory || pods ? {
        cpu: parseInt(cpu) || 0,
        memory: parseInt(memory) || 0,
        pods: parseInt(pods) || 0,
      } : undefined;
      const cost = costTier || costRate ? {
        tier: costTier || 'standard',
        hourlyRate: parseFloat(costRate) || 0,
      } : undefined;
      const healthCheck: HealthCheck | undefined = hcUrl ? {
        url: hcUrl,
        intervalSeconds: parseInt(hcInterval) || undefined,
        timeoutSeconds: parseInt(hcTimeout) || undefined,
        insecureSkipVerify: hcInsecure || undefined,
      } : undefined;

      await environments.create({ name, provider, labels, capabilities, config, resources, cost, healthCheck });
      setName('');
      setProvider('');
      setLabelsStr('');
      setCapsStr('');
      setConfigStr('{}');
      setCpu(''); setMemory(''); setPods('');
      setCostTier(''); setCostRate('');
      setHcUrl(''); setHcInterval(''); setHcTimeout(''); setHcInsecure(false);
      onClose();
      onCreated();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} variant="medium">
      <ModalHeader title="Create Environment" />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <FormGroup label="Name" isRequired fieldId="env-name">
          <TextInput id="env-name" value={name} onChange={(_e, v) => setName(v)} placeholder="k8s-prod-eu" />
        </FormGroup>
        <FormGroup label="Provider type" isRequired fieldId="env-provider" style={{ marginTop: 16 }}>
          <TextInput id="env-provider" value={provider} onChange={(_e, v) => setProvider(v)} placeholder="kubernetes" />
        </FormGroup>
        <FormGroup label="Capabilities (comma-separated)" fieldId="env-caps" style={{ marginTop: 16 }}>
          <TextInput id="env-caps" value={capsStr} onChange={(_e, v) => setCapsStr(v)} placeholder="loadbalancer, persistent-storage, gpu" />
        </FormGroup>
        <FormGroup label="Labels (JSON)" fieldId="env-labels" style={{ marginTop: 16 }}>
          <TextInput id="env-labels" value={labelsStr} onChange={(_e, v) => setLabelsStr(v)} placeholder='{"region": "eu-west-1"}' />
        </FormGroup>
        <FormGroup label="Config (JSON)" fieldId="env-config" style={{ marginTop: 16 }}>
          <TextArea id="env-config" value={configStr} onChange={(_e, v) => setConfigStr(v)} rows={4} style={{ fontFamily: 'monospace', fontSize: 13 }} />
        </FormGroup>
        <FormGroup label="CPU (millicores)" fieldId="env-cpu" style={{ marginTop: 16 }}>
          <TextInput id="env-cpu" type="number" value={cpu} onChange={(_e, v) => setCpu(v)} placeholder="8000" />
        </FormGroup>
        <FormGroup label="Memory (MB)" fieldId="env-memory" style={{ marginTop: 16 }}>
          <TextInput id="env-memory" type="number" value={memory} onChange={(_e, v) => setMemory(v)} placeholder="32768" />
        </FormGroup>
        <FormGroup label="Pods" fieldId="env-pods" style={{ marginTop: 16 }}>
          <TextInput id="env-pods" type="number" value={pods} onChange={(_e, v) => setPods(v)} placeholder="500" />
        </FormGroup>
        <FormGroup label="Cost tier" fieldId="env-cost-tier" style={{ marginTop: 16 }}>
          <TextInput id="env-cost-tier" value={costTier} onChange={(_e, v) => setCostTier(v)} placeholder="standard" />
        </FormGroup>
        <FormGroup label="Hourly rate" fieldId="env-cost-rate" style={{ marginTop: 16 }}>
          <TextInput id="env-cost-rate" type="number" value={costRate} onChange={(_e, v) => setCostRate(v)} placeholder="0.05" />
        </FormGroup>
        <Content component="h3" style={{ marginTop: 24 }}>Health Check</Content>
        <FormGroup label="Probe URL" fieldId="env-hc-url" style={{ marginTop: 8 }}>
          <TextInput id="env-hc-url" value={hcUrl} onChange={(_e, v) => setHcUrl(v)} placeholder="https://cluster:6443/healthz" />
        </FormGroup>
        <FormGroup label="Interval (seconds)" fieldId="env-hc-interval" style={{ marginTop: 16 }}>
          <TextInput id="env-hc-interval" type="number" value={hcInterval} onChange={(_e, v) => setHcInterval(v)} placeholder="30" />
        </FormGroup>
        <FormGroup label="Timeout (seconds)" fieldId="env-hc-timeout" style={{ marginTop: 16 }}>
          <TextInput id="env-hc-timeout" type="number" value={hcTimeout} onChange={(_e, v) => setHcTimeout(v)} placeholder="10" />
        </FormGroup>
        <FormGroup fieldId="env-hc-insecure" style={{ marginTop: 16 }}>
          <label>
            <input type="checkbox" checked={hcInsecure} onChange={e => setHcInsecure(e.target.checked)} />{' '}
            Skip TLS verification
          </label>
        </FormGroup>
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!name || !provider || submitting}>Create</Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}

function EditEnvironmentModal({ env, onClose, onSaved }: { env: EnvironmentRecord | null; onClose: () => void; onSaved: () => void }) {
  const [provider, setProvider] = useState('');
  const [labelsStr, setLabelsStr] = useState('');
  const [capsStr, setCapsStr] = useState('');
  const [configStr, setConfigStr] = useState('{}');
  const [cpu, setCpu] = useState('');
  const [memory, setMemory] = useState('');
  const [pods, setPods] = useState('');
  const [costTier, setCostTier] = useState('');
  const [costRate, setCostRate] = useState('');
  const [hcUrl, setHcUrl] = useState('');
  const [hcInterval, setHcInterval] = useState('');
  const [hcTimeout, setHcTimeout] = useState('');
  const [hcInsecure, setHcInsecure] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (env) {
      setProvider(env.provider);
      setLabelsStr(env.labels && Object.keys(env.labels).length > 0 ? JSON.stringify(env.labels) : '');
      setCapsStr(env.capabilities?.join(', ') ?? '');
      setConfigStr(env.config && Object.keys(env.config).length > 0 ? JSON.stringify(env.config, null, 2) : '{}');
      setCpu(env.resources?.cpu?.toString() ?? '');
      setMemory(env.resources?.memory?.toString() ?? '');
      setPods(env.resources?.pods?.toString() ?? '');
      setCostTier(env.cost?.tier ?? '');
      setCostRate(env.cost?.hourlyRate?.toString() ?? '');
      setHcUrl(env.healthCheck?.url ?? '');
      setHcInterval(env.healthCheck?.intervalSeconds?.toString() ?? '');
      setHcTimeout(env.healthCheck?.timeoutSeconds?.toString() ?? '');
      setHcInsecure(env.healthCheck?.insecureSkipVerify ?? false);
      setError('');
    }
  }, [env]);

  const handleSubmit = async () => {
    if (!env) return;
    setError('');
    setSubmitting(true);
    try {
      let labels: Record<string, string> | undefined;
      if (labelsStr.trim()) labels = JSON.parse(labelsStr);
      const capabilities = capsStr.trim() ? capsStr.split(',').map(s => s.trim()).filter(Boolean) : undefined;
      const config = JSON.parse(configStr);
      const resources = cpu || memory || pods ? {
        cpu: parseInt(cpu) || 0, memory: parseInt(memory) || 0, pods: parseInt(pods) || 0,
      } : undefined;
      const cost = costTier || costRate ? {
        tier: costTier || 'standard', hourlyRate: parseFloat(costRate) || 0,
      } : undefined;
      const healthCheck: HealthCheck | undefined = hcUrl ? {
        url: hcUrl,
        intervalSeconds: parseInt(hcInterval) || undefined,
        timeoutSeconds: parseInt(hcTimeout) || undefined,
        insecureSkipVerify: hcInsecure || undefined,
      } : undefined;

      await environments.update(env.name, { provider, labels, capabilities, config, resources, cost, healthCheck });
      onClose();
      onSaved();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal isOpen={!!env} onClose={onClose} variant="medium">
      <ModalHeader title={`Edit Environment: ${env?.name ?? ''}`} />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <FormGroup label="Name" fieldId="edit-env-name">
          <TextInput id="edit-env-name" value={env?.name ?? ''} isDisabled />
        </FormGroup>
        <FormGroup label="Provider type" isRequired fieldId="edit-env-provider" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-provider" value={provider} onChange={(_e, v) => setProvider(v)} />
        </FormGroup>
        <FormGroup label="Capabilities (comma-separated)" fieldId="edit-env-caps" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-caps" value={capsStr} onChange={(_e, v) => setCapsStr(v)} placeholder="loadbalancer, persistent-storage, gpu" />
        </FormGroup>
        <FormGroup label="Labels (JSON)" fieldId="edit-env-labels" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-labels" value={labelsStr} onChange={(_e, v) => setLabelsStr(v)} placeholder='{"region": "eu-west-1"}' />
        </FormGroup>
        <FormGroup label="Config (JSON)" fieldId="edit-env-config" style={{ marginTop: 16 }}>
          <TextArea id="edit-env-config" value={configStr} onChange={(_e, v) => setConfigStr(v)} rows={4} style={{ fontFamily: 'monospace', fontSize: 13 }} />
        </FormGroup>
        <FormGroup label="CPU (millicores)" fieldId="edit-env-cpu" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-cpu" type="number" value={cpu} onChange={(_e, v) => setCpu(v)} />
        </FormGroup>
        <FormGroup label="Memory (MB)" fieldId="edit-env-memory" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-memory" type="number" value={memory} onChange={(_e, v) => setMemory(v)} />
        </FormGroup>
        <FormGroup label="Pods" fieldId="edit-env-pods" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-pods" type="number" value={pods} onChange={(_e, v) => setPods(v)} />
        </FormGroup>
        <FormGroup label="Cost tier" fieldId="edit-env-cost-tier" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-cost-tier" value={costTier} onChange={(_e, v) => setCostTier(v)} />
        </FormGroup>
        <FormGroup label="Hourly rate" fieldId="edit-env-cost-rate" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-cost-rate" type="number" value={costRate} onChange={(_e, v) => setCostRate(v)} />
        </FormGroup>
        <Content component="h3" style={{ marginTop: 24 }}>Health Check</Content>
        <FormGroup label="Probe URL" fieldId="edit-env-hc-url" style={{ marginTop: 8 }}>
          <TextInput id="edit-env-hc-url" value={hcUrl} onChange={(_e, v) => setHcUrl(v)} placeholder="https://cluster:6443/healthz" />
        </FormGroup>
        <FormGroup label="Interval (seconds)" fieldId="edit-env-hc-interval" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-hc-interval" type="number" value={hcInterval} onChange={(_e, v) => setHcInterval(v)} placeholder="30" />
        </FormGroup>
        <FormGroup label="Timeout (seconds)" fieldId="edit-env-hc-timeout" style={{ marginTop: 16 }}>
          <TextInput id="edit-env-hc-timeout" type="number" value={hcTimeout} onChange={(_e, v) => setHcTimeout(v)} placeholder="10" />
        </FormGroup>
        <FormGroup fieldId="edit-env-hc-insecure" style={{ marginTop: 16 }}>
          <label>
            <input type="checkbox" checked={hcInsecure} onChange={e => setHcInsecure(e.target.checked)} />{' '}
            Skip TLS verification
          </label>
        </FormGroup>
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!provider || submitting}>Save</Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}

function HealthLabel({ status, message, lastHeartbeat }: { status: string; message?: string; lastHeartbeat?: string }) {
  const color = status === 'healthy' ? 'green'
    : status === 'degraded' ? 'orange'
    : status === 'unhealthy' ? 'red'
    : 'grey';

  const label = status || 'unknown';

  const tooltip = [
    message && `Message: ${message}`,
    lastHeartbeat && `Last heartbeat: ${new Date(lastHeartbeat).toLocaleString()}`,
  ].filter(Boolean).join('\n');

  if (tooltip) {
    return (
      <Tooltip content={tooltip}>
        <Label isCompact color={color}>{label}</Label>
      </Tooltip>
    );
  }

  return <Label isCompact color={color}>{label}</Label>;
}
