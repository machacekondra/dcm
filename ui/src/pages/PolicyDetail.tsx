import { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Card,
  CardBody,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Label,
  LabelGroup,
  PageSection,
  Spinner,
} from '@patternfly/react-core';
import { policies, type PolicyRecord } from '../api/client';

export default function PolicyDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [policy, setPolicy] = useState<PolicyRecord | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(() => {
    if (!name) return;
    policies.get(name).then(setPolicy).catch(e => setError(e.message));
  }, [name]);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async () => {
    if (!name || !confirm(`Delete policy "${name}"?`)) return;
    try {
      await policies.delete(name);
      navigate('/policies');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (error) return <PageSection><Alert variant="danger" title={error} /></PageSection>;
  if (!policy) return <PageSection><Spinner /></PageSection>;

  return (
    <>
      <PageSection variant="light">
        <Breadcrumb style={{ marginBottom: 16 }}>
          <BreadcrumbItem to="/policies" onClick={e => { e.preventDefault(); navigate('/policies'); }}>
            Policies
          </BreadcrumbItem>
          <BreadcrumbItem isActive>{name}</BreadcrumbItem>
        </Breadcrumb>

        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>
            <Content component="h1">{policy.name}</Content>
          </FlexItem>
          <FlexItem>
            <Button variant="danger" onClick={handleDelete}>Delete</Button>
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection>
      <Card style={{ marginBottom: 24 }}>
        <CardTitle>Details</CardTitle>
        <CardBody>
          <DescriptionList isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Rules</DescriptionListTerm>
              <DescriptionListDescription>{policy.rules.length}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Created</DescriptionListTerm>
              <DescriptionListDescription>{new Date(policy.createdAt).toLocaleString()}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Updated</DescriptionListTerm>
              <DescriptionListDescription>{new Date(policy.updatedAt).toLocaleString()}</DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        </CardBody>
      </Card>

      {policy.rules.map((rule, i) => (
        <Card key={i} style={{ marginBottom: 16 }}>
          <CardTitle>{rule.name || `Rule ${i + 1}`}</CardTitle>
          <CardBody>
            <DescriptionList isHorizontal columnModifier={{ default: '2Col' }}>
              <DescriptionListGroup>
                <DescriptionListTerm>Priority</DescriptionListTerm>
                <DescriptionListDescription>{rule.priority ?? 0}</DescriptionListDescription>
              </DescriptionListGroup>

              {rule.match.type && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Match type</DescriptionListTerm>
                  <DescriptionListDescription><Label isCompact>{rule.match.type}</Label></DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.match.labels && Object.keys(rule.match.labels).length > 0 && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Match labels</DescriptionListTerm>
                  <DescriptionListDescription>
                    <LabelGroup>
                      {Object.entries(rule.match.labels).map(([k, v]) => (
                        <Label key={k} isCompact>{k}={v}</Label>
                      ))}
                    </LabelGroup>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.match.expression && (
                <DescriptionListGroup>
                  <DescriptionListTerm>CEL expression</DescriptionListTerm>
                  <DescriptionListDescription>
                    <code style={{ fontSize: 13 }}>{rule.match.expression}</code>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.providers.required && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Required</DescriptionListTerm>
                  <DescriptionListDescription><Label color="red" isCompact>{rule.providers.required}</Label></DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.providers.preferred && rule.providers.preferred.length > 0 && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Preferred</DescriptionListTerm>
                  <DescriptionListDescription>
                    <LabelGroup>
                      {rule.providers.preferred.map(p => <Label key={p} color="blue" isCompact>{p}</Label>)}
                    </LabelGroup>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.providers.forbidden && rule.providers.forbidden.length > 0 && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Forbidden</DescriptionListTerm>
                  <DescriptionListDescription>
                    <LabelGroup>
                      {rule.providers.forbidden.map(p => <Label key={p} color="red" isCompact>{p}</Label>)}
                    </LabelGroup>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.providers.strategy && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Strategy</DescriptionListTerm>
                  <DescriptionListDescription>{rule.providers.strategy}</DescriptionListDescription>
                </DescriptionListGroup>
              )}

              {rule.properties && Object.keys(rule.properties).length > 0 && (
                <DescriptionListGroup>
                  <DescriptionListTerm>Injected properties</DescriptionListTerm>
                  <DescriptionListDescription>
                    <code style={{ fontSize: 12 }}>{JSON.stringify(rule.properties)}</code>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}
            </DescriptionList>
          </CardBody>
        </Card>
      ))}
    </PageSection>
    </>
  );
}
