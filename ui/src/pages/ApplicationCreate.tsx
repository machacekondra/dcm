import { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Card,
  CardBody,
  CardTitle,
  Content,
  Flex,
  FlexItem,
  FormGroup,
  Label,
  LabelGroup,
  MenuToggle,
  NumberInput,
  PageSection,
  Select,
  SelectList,
  SelectOption,
  Spinner,
  Switch,
  TextInput,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { TrashIcon, PlusCircleIcon } from '@patternfly/react-icons';
import {
  applications,
  types as typesApi,
  deployments,
  type Component,
  type TypeSchema,
} from '../api/client';

interface ComponentDraft {
  id: string;
  name: string;
  type: string;
  dependsOn: string[];
  labels: Record<string, string>;
  properties: Record<string, unknown>;
}

let nextId = 1;
function makeId() {
  return `comp-${nextId++}`;
}

export default function ApplicationCreate() {
  const navigate = useNavigate();
  const [appName, setAppName] = useState('');
  const [appLabelsStr, setAppLabelsStr] = useState('');
  const [components, setComponents] = useState<ComponentDraft[]>([]);
  const [typeSchemas, setTypeSchemas] = useState<TypeSchema[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [validating, setValidating] = useState(false);
  const [planning, setPlanning] = useState(false);
  const [planResult, setPlanResult] = useState<any>(null);
  const [validateResult, setValidateResult] = useState<{ valid: boolean; errors?: string[] } | null>(null);

  // Editing state
  const [editingIdx, setEditingIdx] = useState<number | null>(null);

  useEffect(() => {
    typesApi.list()
      .then(setTypeSchemas)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const buildComponents = useCallback((): Component[] => {
    return components.map(c => {
      const comp: Component = { name: c.name, type: c.type };
      if (c.dependsOn.length > 0) comp.dependsOn = c.dependsOn;
      if (Object.keys(c.labels).length > 0) comp.labels = c.labels;
      if (Object.keys(c.properties).length > 0) comp.properties = c.properties;
      return comp;
    });
  }, [components]);

  const addComponent = (schema: TypeSchema) => {
    const baseName = schema.type.replace('-', '_');
    let name = baseName;
    let i = 1;
    while (components.some(c => c.name === name)) {
      name = `${baseName}_${i++}`;
    }

    // Pre-fill defaults
    const properties: Record<string, unknown> = {};
    for (const prop of schema.properties) {
      if (prop.default !== undefined) {
        properties[prop.name] = prop.default;
      }
    }

    setComponents(prev => [...prev, {
      id: makeId(),
      name,
      type: schema.type,
      dependsOn: [],
      labels: {},
      properties,
    }]);
    setEditingIdx(components.length);
    clearResults();
  };

  const removeComponent = (idx: number) => {
    const removed = components[idx].name;
    setComponents(prev => {
      const next = prev.filter((_, i) => i !== idx);
      // Clean up dependency references
      return next.map(c => ({
        ...c,
        dependsOn: c.dependsOn.filter(d => d !== removed),
      }));
    });
    if (editingIdx === idx) setEditingIdx(null);
    else if (editingIdx !== null && editingIdx > idx) setEditingIdx(editingIdx - 1);
    clearResults();
  };

  const updateComponent = (idx: number, updates: Partial<ComponentDraft>) => {
    setComponents(prev => prev.map((c, i) => i === idx ? { ...c, ...updates } : c));
    clearResults();
  };

  const clearResults = () => {
    setValidateResult(null);
    setPlanResult(null);
    setError('');
    setSuccess('');
  };

  const handleCreate = async () => {
    if (!appName.trim()) { setError('Application name is required'); return; }
    if (components.length === 0) { setError('Add at least one component'); return; }
    setError('');
    setSubmitting(true);
    try {
      let labels: Record<string, string> | undefined;
      if (appLabelsStr.trim()) labels = JSON.parse(appLabelsStr);
      await applications.create({ name: appName, labels, components: buildComponents() });
      navigate(`/applications/${appName}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const handleValidate = async () => {
    if (!appName.trim()) { setError('Application name is required'); return; }
    if (components.length === 0) { setError('Add at least one component'); return; }
    setError('');
    setValidating(true);
    setValidateResult(null);
    try {
      let labels: Record<string, string> | undefined;
      if (appLabelsStr.trim()) labels = JSON.parse(appLabelsStr);
      // Create temporarily, validate, then delete
      await applications.create({ name: appName, labels, components: buildComponents() });
      try {
        const result = await applications.validate(appName);
        setValidateResult(result);
      } finally {
        await applications.delete(appName).catch(() => {});
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setValidating(false);
    }
  };

  const handlePlan = async () => {
    if (!appName.trim()) { setError('Application name is required'); return; }
    if (components.length === 0) { setError('Add at least one component'); return; }
    setError('');
    setPlanning(true);
    setPlanResult(null);
    try {
      let labels: Record<string, string> | undefined;
      if (appLabelsStr.trim()) labels = JSON.parse(appLabelsStr);
      // Create temporarily to run dry-run deployment
      await applications.create({ name: appName, labels, components: buildComponents() });
      try {
        const dep = await deployments.create({ application: appName, dryRun: true });
        setPlanResult(dep.plan);
      } finally {
        await applications.delete(appName).catch(() => {});
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setPlanning(false);
    }
  };

  if (loading) return <PageSection><Spinner /></PageSection>;

  const schema = editingIdx !== null ? typeSchemas.find(t => t.type === components[editingIdx]?.type) : null;

  return (
    <>
      <PageSection variant="light">
        <Breadcrumb style={{ marginBottom: 16 }}>
          <BreadcrumbItem to="/applications" onClick={e => { e.preventDefault(); navigate('/applications'); }}>
            Applications
          </BreadcrumbItem>
          <BreadcrumbItem isActive>Create</BreadcrumbItem>
        </Breadcrumb>
        <Content component="h1">Create Application</Content>
      </PageSection>
      <PageSection>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        {success && <Alert variant="success" title={success} isInline style={{ marginBottom: 16 }} />}
        {validateResult && (
          <Alert
            variant={validateResult.valid ? 'success' : 'danger'}
            title={validateResult.valid ? 'Validation passed' : 'Validation failed'}
            isInline
            style={{ marginBottom: 16 }}
          >
            {validateResult.errors?.map((e, i) => <div key={i}>{e}</div>)}
          </Alert>
        )}

        {/* App metadata */}
        <Card style={{ marginBottom: 24 }}>
          <CardTitle>Application</CardTitle>
          <CardBody>
            <Flex direction={{ default: 'column' }} gap={{ default: 'gapMd' }}>
              <FlexItem>
                <FormGroup label="Name" isRequired fieldId="app-name">
                  <TextInput id="app-name" value={appName} onChange={(_e, v) => { setAppName(v); clearResults(); }} placeholder="my-web-app" />
                </FormGroup>
              </FlexItem>
              <FlexItem>
                <FormGroup label="Labels (JSON, optional)" fieldId="app-labels">
                  <TextInput id="app-labels" value={appLabelsStr} onChange={(_e, v) => { setAppLabelsStr(v); clearResults(); }} placeholder='{"env": "production"}' />
                </FormGroup>
              </FlexItem>
            </Flex>
          </CardBody>
        </Card>

        {/* Components */}
        <Card style={{ marginBottom: 24 }}>
          <CardTitle>
            <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
              <FlexItem>Components ({components.length})</FlexItem>
            </Flex>
          </CardTitle>
          <CardBody>
            {/* Type selector */}
            <Content component="h4" style={{ marginBottom: 8 }}>Add a component</Content>
            <Flex gap={{ default: 'gapSm' }} style={{ marginBottom: 24 }} wrap={{ default: 'wrap' }}>
              {typeSchemas.map(ts => (
                <FlexItem key={ts.type}>
                  <Button
                    variant="secondary"
                    icon={<PlusCircleIcon />}
                    onClick={() => addComponent(ts)}
                  >
                    {ts.type}
                  </Button>
                </FlexItem>
              ))}
            </Flex>

            {components.length === 0 ? (
              <Content component="p" style={{ color: '#6a6e73' }}>
                No components yet. Click a type above to add one.
              </Content>
            ) : (
              <Table aria-label="Components" variant="compact">
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    <Th>Type</Th>
                    <Th>Dependencies</Th>
                    <Th>Properties</Th>
                    <Th />
                  </Tr>
                </Thead>
                <Tbody>
                  {components.map((comp, idx) => (
                    <Tr
                      key={comp.id}
                      isClickable
                      isRowSelected={editingIdx === idx}
                      onRowClick={() => setEditingIdx(editingIdx === idx ? null : idx)}
                    >
                      <Td dataLabel="Name"><strong>{comp.name}</strong></Td>
                      <Td dataLabel="Type"><Label isCompact>{comp.type}</Label></Td>
                      <Td dataLabel="Dependencies">
                        {comp.dependsOn.length > 0 ? (
                          <LabelGroup>
                            {comp.dependsOn.map(d => <Label key={d} isCompact color="blue">{d}</Label>)}
                          </LabelGroup>
                        ) : '—'}
                      </Td>
                      <Td dataLabel="Properties">
                        <code style={{ fontSize: 12 }}>
                          {Object.entries(comp.properties).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : v}`).join(', ') || '—'}
                        </code>
                      </Td>
                      <Td isActionCell>
                        <Button
                          variant="plain"
                          aria-label="Remove"
                          onClick={e => { e.stopPropagation(); removeComponent(idx); }}
                        >
                          <TrashIcon />
                        </Button>
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </CardBody>
        </Card>

        {/* Component editor */}
        {editingIdx !== null && components[editingIdx] && schema && (
          <ComponentEditor
            comp={components[editingIdx]}
            schema={schema}
            allComponents={components}
            onChange={updates => updateComponent(editingIdx, updates)}
          />
        )}

        {/* Plan result */}
        {planResult && (
          <Card style={{ marginBottom: 24 }}>
            <CardTitle>Plan Result ({planResult.steps?.length || 0} steps)</CardTitle>
            <CardBody>
              <Table aria-label="Plan" variant="compact">
                <Thead>
                  <Tr>
                    <Th>Component</Th>
                    <Th>Action</Th>
                    <Th>Type</Th>
                    <Th>Provider</Th>
                    <Th>Environment</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {planResult.steps?.map((step: any) => (
                    <Tr key={step.component}>
                      <Td><strong>{step.component}</strong></Td>
                      <Td>
                        <Label isCompact color={step.diff.action === 'create' ? 'green' : step.diff.action === 'delete' ? 'red' : 'blue'}>
                          {step.diff.action}
                        </Label>
                      </Td>
                      <Td>{step.diff.type}</Td>
                      <Td>{step.diff.provider}</Td>
                      <Td>{step.diff.environment || '—'}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        )}

        {/* Action buttons */}
        <Flex gap={{ default: 'gapMd' }}>
          <FlexItem>
            <Button
              onClick={handleCreate}
              isLoading={submitting}
              isDisabled={!appName || components.length === 0 || submitting}
            >
              Create
            </Button>
          </FlexItem>
          <FlexItem>
            <Button
              variant="secondary"
              onClick={handleValidate}
              isLoading={validating}
              isDisabled={!appName || components.length === 0 || validating}
            >
              Validate
            </Button>
          </FlexItem>
          <FlexItem>
            <Button
              variant="secondary"
              onClick={handlePlan}
              isLoading={planning}
              isDisabled={!appName || components.length === 0 || planning}
            >
              Dry Run (Plan)
            </Button>
          </FlexItem>
          <FlexItem>
            <Button variant="link" onClick={() => navigate('/applications')}>Cancel</Button>
          </FlexItem>
        </Flex>
      </PageSection>
    </>
  );
}

function ComponentEditor({
  comp,
  schema,
  allComponents,
  onChange,
}: {
  comp: ComponentDraft;
  schema: TypeSchema;
  allComponents: ComponentDraft[];
  onChange: (updates: Partial<ComponentDraft>) => void;
}) {
  const [depsOpen, setDepsOpen] = useState(false);
  const otherComponents = allComponents.filter(c => c.id !== comp.id);

  const setProp = (name: string, value: unknown) => {
    onChange({ properties: { ...comp.properties, [name]: value } });
  };

  const toggleDep = (name: string) => {
    const next = comp.dependsOn.includes(name)
      ? comp.dependsOn.filter(d => d !== name)
      : [...comp.dependsOn, name];
    onChange({ dependsOn: next });
  };

  return (
    <Card style={{ marginBottom: 24 }}>
      <CardTitle>
        <Flex gap={{ default: 'gapSm' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>Edit: {comp.name}</FlexItem>
          <FlexItem><Label isCompact>{comp.type}</Label></FlexItem>
          <FlexItem><Content component="small" style={{ color: '#6a6e73' }}>{schema.description}</Content></FlexItem>
        </Flex>
      </CardTitle>
      <CardBody>
        <Flex direction={{ default: 'column' }} gap={{ default: 'gapMd' }}>
          {/* Component name */}
          <FlexItem>
            <FormGroup label="Component name" isRequired fieldId={`${comp.id}-name`}>
              <TextInput
                id={`${comp.id}-name`}
                value={comp.name}
                onChange={(_e, v) => onChange({ name: v })}
              />
            </FormGroup>
          </FlexItem>

          {/* Type-specific properties */}
          {schema.properties.map(prop => (
            <FlexItem key={prop.name}>
              <FormGroup
                label={
                  <span>
                    {prop.name}
                    {prop.required && <span style={{ color: '#c9190b' }}> *</span>}
                    {prop.description && <span style={{ color: '#6a6e73', fontWeight: 'normal', marginLeft: 8, fontSize: 13 }}>{prop.description}</span>}
                  </span>
                }
                fieldId={`${comp.id}-${prop.name}`}
              >
                <PropertyInput
                  id={`${comp.id}-${prop.name}`}
                  schema={prop}
                  value={comp.properties[prop.name]}
                  onChange={v => setProp(prop.name, v)}
                />
              </FormGroup>
            </FlexItem>
          ))}

          {/* Dependencies */}
          {otherComponents.length > 0 && (
            <FlexItem>
              <FormGroup label="Depends on" fieldId={`${comp.id}-deps`}>
                <Select
                  id={`${comp.id}-deps`}
                  isOpen={depsOpen}
                  onOpenChange={setDepsOpen}
                  selected={comp.dependsOn}
                  onSelect={(_e, val) => toggleDep(val as string)}
                  toggle={(toggleRef) => (
                    <MenuToggle ref={toggleRef} onClick={() => setDepsOpen(!depsOpen)} style={{ width: '100%' }}>
                      {comp.dependsOn.length > 0 ? comp.dependsOn.join(', ') : 'None'}
                    </MenuToggle>
                  )}
                >
                  <SelectList>
                    {otherComponents.map(c => (
                      <SelectOption
                        key={c.name}
                        value={c.name}
                        hasCheckbox
                        isSelected={comp.dependsOn.includes(c.name)}
                      >
                        {c.name} ({c.type})
                      </SelectOption>
                    ))}
                  </SelectList>
                </Select>
              </FormGroup>
            </FlexItem>
          )}

          {/* Labels */}
          <FlexItem>
            <FormGroup label="Labels (optional, JSON)" fieldId={`${comp.id}-labels`}>
              <TextInput
                id={`${comp.id}-labels`}
                value={Object.keys(comp.labels).length > 0 ? JSON.stringify(comp.labels) : ''}
                onChange={(_e, v) => {
                  try {
                    if (v.trim()) onChange({ labels: JSON.parse(v) });
                    else onChange({ labels: {} });
                  } catch { /* ignore parse errors while typing */ }
                }}
                placeholder='{"tier": "frontend"}'
              />
            </FormGroup>
          </FlexItem>
        </Flex>
      </CardBody>
    </Card>
  );
}

function PropertyInput({
  id,
  schema,
  value,
  onChange,
}: {
  id: string;
  schema: { name: string; type: string; default?: unknown };
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  switch (schema.type) {
    case 'number':
      return (
        <NumberInput
          id={id}
          value={typeof value === 'number' ? value : (schema.default as number) ?? 0}
          onChange={(e: React.FormEvent<HTMLInputElement>) => {
            const v = parseInt((e.target as HTMLInputElement).value, 10);
            if (!isNaN(v)) onChange(v);
          }}
          onMinus={() => onChange(Math.max(0, (typeof value === 'number' ? value : 0) - 1))}
          onPlus={() => onChange((typeof value === 'number' ? value : 0) + 1)}
          min={0}
        />
      );
    case 'boolean':
      return (
        <Switch
          id={id}
          isChecked={typeof value === 'boolean' ? value : (schema.default as boolean) ?? false}
          onChange={(_e, checked) => onChange(checked)}
        />
      );
    case 'object':
      return (
        <TextInput
          id={id}
          value={value && typeof value === 'object' ? JSON.stringify(value) : (value as string) ?? ''}
          onChange={(_e, v) => {
            try {
              if (v.trim()) onChange(JSON.parse(v));
              else onChange(undefined);
            } catch { /* ignore parse errors while typing */ }
          }}
          placeholder='{"KEY": "value"}'
        />
      );
    default: // string
      return (
        <TextInput
          id={id}
          value={(value as string) ?? ''}
          onChange={(_e, v) => onChange(v)}
          placeholder={schema.default ? String(schema.default) : undefined}
        />
      );
  }
}
