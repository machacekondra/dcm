import { Label } from '@patternfly/react-core';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InProgressIcon,
  PendingIcon,
  TrashIcon,
} from '@patternfly/react-icons';

const statusConfig: Record<string, { color: 'green' | 'red' | 'blue' | 'orange' | 'grey' | 'cyan'; icon: React.ComponentType }> = {
  ready: { color: 'green', icon: CheckCircleIcon },
  failed: { color: 'red', icon: ExclamationCircleIcon },
  deploying: { color: 'blue', icon: InProgressIcon },
  planning: { color: 'blue', icon: InProgressIcon },
  pending: { color: 'grey', icon: PendingIcon },
  planned: { color: 'cyan', icon: CheckCircleIcon },
  rehydrating: { color: 'orange', icon: InProgressIcon },
  destroying: { color: 'orange', icon: InProgressIcon },
  destroyed: { color: 'grey', icon: TrashIcon },
};

export default function StatusLabel({ status }: { status: string }) {
  const cfg = statusConfig[status] || { color: 'grey' as const, icon: PendingIcon };
  return <Label color={cfg.color} icon={<cfg.icon />}>{status}</Label>;
}
