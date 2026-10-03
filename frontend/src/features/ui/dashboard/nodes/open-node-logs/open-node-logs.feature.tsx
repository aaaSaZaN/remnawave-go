import { ActionIcon, Tooltip } from '@mantine/core'
import { GetNodeCommand } from '@remnawave/backend-contract'
import { memo } from 'react'
import { TbTerminal2 } from 'react-icons/tb'

import { showModal } from '@shared/_modals/show-modal'

interface IProps {
    node: GetNodeCommand.Response['response']
}

const OpenNodeLogsFeatureComponent = (props: IProps) => {
    const { node } = props

    return (
        <Tooltip label="Логи ноды / Xray">
            <ActionIcon
                color="cyan"
                onClick={() => {
                    showModal('nodes_nodeLogsModal', { node })
                }}
                size="lg"
                variant="soft"
            >
                <TbTerminal2 size={22} />
            </ActionIcon>
        </Tooltip>
    )
}

export const OpenNodeLogsFeature = memo(OpenNodeLogsFeatureComponent)
