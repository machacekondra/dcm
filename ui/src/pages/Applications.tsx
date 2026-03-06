import { useEffect, useState, useCallback } from 'react';
import {
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
  Alert,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useNavigate } from 'react-router-dom';
import { applications, type ApplicationRecord, type Component } from '../api/client';

export default function Applications() {
  const [apps, setApps] = useState<ApplicationRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const load = useCallback(() => {
    setLoading(true);
    applications.list().then(setApps).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete application "${name}"?`)) return;
    try {
      await applications.delete(name);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Applications</Content>
      </PageSection>
      <PageSection>
      {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button onClick={() => setCreateOpen(true)}>Create application</Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
      {!loading && apps.length === 0 ? (
        <EmptyState>
          <EmptyStateBody>No applications yet.</EmptyStateBody>
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={() => setCreateOpen(true)}>Create application</Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      ) : (
        <Table aria-label="Applications">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Labels</Th>
              <Th>Components</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {apps.map(app => (
              <Tr
                key={app.name}
                onRowClick={() => navigate(`/applications/${app.name}`)}
                isClickable
              >
                <Td dataLabel="Name">{app.name}</Td>
                <Td dataLabel="Labels">
                  {app.labels ? Object.entries(app.labels).map(([k, v]) => `${k}=${v}`).join(', ') : '—'}
                </Td>
                <Td dataLabel="Components">{app.components.length}</Td>
                <Td dataLabel="Created">{new Date(app.createdAt).toLocaleString()}</Td>
                <Td dataLabel="Actions" isActionCell>
                  <Button variant="danger" size="sm" onClick={e => { e.stopPropagation(); handleDelete(app.name); }}>
                    Delete
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
      <CreateApplicationModal isOpen={isCreateOpen} onClose={() => setCreateOpen(false)} onCreated={load} />
    </PageSection>
    </>
  );
}

function CreateApplicationModal({ isOpen, onClose, onCreated }: { isOpen: boolean; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [labelsStr, setLabelsStr] = useState('');
  const [componentsStr, setComponentsStr] = useState(
    JSON.stringify([{ name: 'web', type: 'container', properties: { image: 'nginx:latest' } }], null, 2)
  );
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
      const components: Component[] = JSON.parse(componentsStr);
      await applications.create({ name, labels, components });
      setName('');
      setLabelsStr('');
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
      <ModalHeader title="Create Application" />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <FormGroup label="Name" isRequired fieldId="app-name">
          <TextInput id="app-name" value={name} onChange={(_e, v) => setName(v)} placeholder="my-web-app" />
        </FormGroup>
        <FormGroup label="Labels (JSON)" fieldId="app-labels" style={{ marginTop: 16 }}>
          <TextInput id="app-labels" value={labelsStr} onChange={(_e, v) => setLabelsStr(v)} placeholder='{"env": "production"}' />
        </FormGroup>
        <FormGroup label="Components (JSON)" isRequired fieldId="app-components" style={{ marginTop: 16 }}>
          <TextArea id="app-components" value={componentsStr} onChange={(_e, v) => setComponentsStr(v)} rows={12} style={{ fontFamily: 'monospace', fontSize: 13 }} />
        </FormGroup>
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!name || submitting}>Create</Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}
