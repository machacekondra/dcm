import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  FormGroup,
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
import { useNavigate } from 'react-router-dom';
import { policies, type PolicyRecord, type PolicyRule } from '../api/client';

const EXAMPLE_RULES: PolicyRule[] = [
  {
    name: 'prefer-mock',
    priority: 100,
    match: { type: 'container' },
    providers: { preferred: ['mock'] },
  },
];

export default function Policies() {
  const [list, setList] = useState<PolicyRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const load = useCallback(() => {
    setLoading(true);
    policies.list().then(setList).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete policy "${name}"?`)) return;
    try {
      await policies.delete(name);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Policies</Content>
      </PageSection>
      <PageSection>
      {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button onClick={() => setCreateOpen(true)}>Create policy</Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
      {!loading && list.length === 0 ? (
        <EmptyState>
          <EmptyStateBody>No policies defined.</EmptyStateBody>
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={() => setCreateOpen(true)}>Create policy</Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      ) : (
        <Table aria-label="Policies">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Rules</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {list.map(p => (
              <Tr key={p.name} onRowClick={() => navigate(`/policies/${p.name}`)} isClickable>
                <Td dataLabel="Name">{p.name}</Td>
                <Td dataLabel="Rules">{p.rules.length}</Td>
                <Td dataLabel="Created">{new Date(p.createdAt).toLocaleString()}</Td>
                <Td dataLabel="Actions" isActionCell>
                  <Button variant="danger" size="sm" onClick={e => { e.stopPropagation(); handleDelete(p.name); }}>
                    Delete
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
      <CreatePolicyModal isOpen={isCreateOpen} onClose={() => setCreateOpen(false)} onCreated={load} />
    </PageSection>
    </>
  );
}

function CreatePolicyModal({ isOpen, onClose, onCreated }: { isOpen: boolean; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [rulesStr, setRulesStr] = useState(JSON.stringify(EXAMPLE_RULES, null, 2));
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async () => {
    setError('');
    setSubmitting(true);
    try {
      const rules: PolicyRule[] = JSON.parse(rulesStr);
      await policies.create({ name, rules });
      setName('');
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
      <ModalHeader title="Create Policy" />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <FormGroup label="Name" isRequired fieldId="policy-name">
          <TextInput id="policy-name" value={name} onChange={(_e, v) => setName(v)} placeholder="production-placement" />
        </FormGroup>
        <FormGroup label="Rules (JSON)" isRequired fieldId="policy-rules" style={{ marginTop: 16 }}>
          <TextArea id="policy-rules" value={rulesStr} onChange={(_e, v) => setRulesStr(v)} rows={16} style={{ fontFamily: 'monospace', fontSize: 13 }} />
        </FormGroup>
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!name || submitting}>Create</Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}
