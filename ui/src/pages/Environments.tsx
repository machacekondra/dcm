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
import { environments, type EnvironmentRecord } from '../api/client';

export default function Environments() {
  const [list, setList] = useState<EnvironmentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);
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
                <Th>Labels</Th>
                <Th>Resources</Th>
                <Th>Cost</Th>
                <Th>Status</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {list.map(env => (
                <Tr key={env.name}>
                  <Td dataLabel="Name"><strong>{env.name}</strong></Td>
                  <Td dataLabel="Provider"><Label isCompact color="blue">{env.provider}</Label></Td>
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
                  <Td dataLabel="Actions" isActionCell>
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
      </PageSection>
    </>
  );
}

function CreateEnvironmentModal({ isOpen, onClose, onCreated }: { isOpen: boolean; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [provider, setProvider] = useState('');
  const [labelsStr, setLabelsStr] = useState('');
  const [configStr, setConfigStr] = useState('{}');
  const [cpu, setCpu] = useState('');
  const [memory, setMemory] = useState('');
  const [pods, setPods] = useState('');
  const [costTier, setCostTier] = useState('');
  const [costRate, setCostRate] = useState('');
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

      await environments.create({ name, provider, labels, config, resources, cost });
      setName('');
      setProvider('');
      setLabelsStr('');
      setConfigStr('{}');
      setCpu(''); setMemory(''); setPods('');
      setCostTier(''); setCostRate('');
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
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!name || !provider || submitting}>Create</Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}
