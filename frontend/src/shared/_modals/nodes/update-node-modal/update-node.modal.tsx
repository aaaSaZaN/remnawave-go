import NiceModal, { useModal } from '@ebay/nice-modal-react'
import {
    ActionIcon,
    Alert,
    Badge,
    Box,
    Button,
    Checkbox,
    Divider,
    Group,
    Loader,
    Modal,
    Select,
    Stack,
    Switch,
    Text,
    TextInput,
    Tooltip
} from '@mantine/core'
import { GetNodeCommand } from '@remnawave/backend-contract'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
    TbAlertTriangle,
    TbArrowRight,
    TbCheck,
    TbCloudDownload,
    TbCpu,
    TbRefresh
} from 'react-icons/tb'

import { useNiceMantineModal } from '@shared/_modals/use-nice-modal'
import { instance } from '@shared/api/axios'
import { BaseOverlayHeader } from '@shared/ui/overlays/base-overlay-header'

interface IProps {
    node: GetNodeCommand.Response['response']
}

interface IReleaseInfo {
    tag_name: string
    name: string
    prerelease: boolean
    published_at: string
}

interface IUpdatesCheckResponse {
    currentVersion: string
    latestRelease?: IReleaseInfo
    latestPrerelease?: IReleaseInfo
    allReleases: IReleaseInfo[]
    architecture: string
    os: string
    currentCoreVersion: string
}

export const UpdateNodeModal = NiceModal.create((props: IProps) => {
    const { node } = props
    const modal = useModal()
    const { modalProps, hide } = useNiceMantineModal({ modal })

    const [channel, setChannel] = useState<'stable' | 'prerelease' | 'custom' | 'manual'>('stable')
    const [selectedVersion, setSelectedVersion] = useState<string>('')
    const [manualVersion, setManualVersion] = useState<string>('')
    const [confirmed, setConfirmed] = useState<boolean>(false)

    const isGoNode =
        (node as any)?.versions?.nodeType === "go" ||
        (node as any)?.versions?.node?.includes("(Go)")

    const [nodeRepo, setNodeRepo] = useState<string>(
        isGoNode ? "aaaSaZaN/remnanode-go" : "remnawave/node"
    )

    const [isCustomCoreEnabled, setIsCustomCoreEnabled] = useState<boolean>(false)
    const [customCoreRepo, setCustomCoreRepo] = useState<string>('XTLS/Xray-core')
    const [customCoreVersion, setCustomCoreVersion] = useState<string>('')

    const {
        data: updateInfo,
        isLoading,
        refetch
    } = useQuery({
        queryKey: ['node-updates-check', node.uuid, nodeRepo],
        queryFn: async () => {
            const res = await instance.get<{ response: IUpdatesCheckResponse }>(
                `/api/nodes/${node.uuid}/updates` as any,
                { params: { repo: nodeRepo } }
            )
            return res.data.response
        },
        refetchOnWindowFocus: false
    })

    const applyMutation = useMutation({
        mutationFn: async () => {
            let targetVer = selectedVersion
            if (channel === 'stable' && updateInfo?.latestRelease) {
                targetVer = updateInfo.latestRelease.tag_name
            } else if (channel === 'prerelease') {
                targetVer = updateInfo?.latestPrerelease?.tag_name || 'v3.5.0-beta.1'
            } else if (channel === 'manual' || selectedVersion === 'manual') {
                targetVer = manualVersion
            }

            if (isCustomCoreEnabled) {
                await instance.post(`/api/nodes/${node.uuid}/updates/apply` as any, {
                    type: 'core',
                    repo: customCoreRepo,
                    version: customCoreVersion
                })
            }

            const res = await instance.post(`/api/nodes/${node.uuid}/updates/apply` as any, {
                type: 'node',
                repo: nodeRepo,
                version: targetVer
            })
            return res.data
        },
        onSuccess: () => {
            setTimeout(() => {
                hide()
            }, 1500)
        }
    })

    const effectiveTargetVer =
        channel === 'stable'
            ? updateInfo?.latestRelease?.tag_name || 'Latest Stable'
            : channel === 'prerelease'
              ? updateInfo?.latestPrerelease?.tag_name || 'v3.5.0-beta.1 (Pre-release)'
              : channel === 'manual' || selectedVersion === 'manual'
                ? manualVersion || 'Ввести версию'
                : selectedVersion || 'Выбрать'

    const allReleases = updateInfo?.allReleases || []
    const prereleasesFromApi = allReleases.filter(
        (r) =>
            r.prerelease ||
            r.tag_name.toLowerCase().includes('beta') ||
            r.tag_name.toLowerCase().includes('rc') ||
            r.tag_name.toLowerCase().includes('pre') ||
            r.tag_name.toLowerCase().includes('alpha') ||
            r.tag_name.toLowerCase().includes('dev')
    )

    const prereleaseOptions =
        prereleasesFromApi.length > 0
            ? prereleasesFromApi.map((r) => ({
                  value: r.tag_name,
                  label: `${r.tag_name} (Pre-release)`
              }))
            : [
                  { value: 'v3.5.0-rc.1', label: 'v3.5.0-rc.1 (Pre-release)' },
                  { value: 'v3.5.0-beta.1', label: 'v3.5.0-beta.1 (Pre-release)' }
              ]

    const stableReleases = allReleases
        .filter(
            (r) =>
                !r.prerelease &&
                !r.tag_name.toLowerCase().includes('beta') &&
                !r.tag_name.toLowerCase().includes('rc') &&
                !r.tag_name.toLowerCase().includes('pre') &&
                !r.tag_name.toLowerCase().includes('alpha') &&
                !r.tag_name.toLowerCase().includes('dev')
        )
        .map((r) => ({
            value: r.tag_name,
            label: `${r.tag_name} (Release)`
        }))

    const releaseOptions = [
        {
            group: 'Pre-release',
            items: prereleaseOptions
        },
        {
            group: 'Release',
            items: stableReleases
        },
        {
            group: 'Другое',
            items: [{ value: 'manual', label: '✏️ Ввести другую версию вручную...' }]
        }
    ]

    return (
        <Modal
            {...modalProps}
            centered
            size="min(640px, 95vw)"
            title={
                <BaseOverlayHeader
                    iconColor="blue"
                    IconComponent={TbCloudDownload}
                    iconVariant="soft"
                    subtitle={node.name}
                    title="Обновление Remnanode"
                />
            }
        >
            <Stack gap="md">
                <Alert
                    color="orange"
                    icon={<TbAlertTriangle size={20} />}
                    radius="md"
                    title="Внимание"
                    variant="light"
                >
                    Обновление бинарника приведет к перезапуску службы Remnanode. Активные соединения пользователей будут кратковременно сброшены.
                </Alert>

                {isLoading ? (
                    <Group justify="center" p="xl">
                        <Loader color="blue" size="sm" />
                        <Text c="dimmed" size="xs">
                            Проверка доступных релизов...
                        </Text>
                    </Group>
                ) : (
                    <>
                        <Group justify="space-between">
                            <Group gap="xs">
                                <Text fw={500} size="sm">
                                    Текущая версия:
                                </Text>
                                <Badge color="gray" size="md" variant="light">
                                    v{updateInfo?.currentVersion || '3.4.15'}
                                </Badge>
                                <Badge color="dark" size="sm" variant="outline">
                                    {updateInfo?.os}/{updateInfo?.architecture}
                                </Badge>
                            </Group>

                            <Group gap="xs">
                                <Text c="dimmed" size="xs">
                                    Xray Core:
                                </Text>
                                <Badge color="teal" size="sm" variant="light">
                                    {updateInfo?.currentCoreVersion || 'v26.3.27'}
                                </Badge>
                            </Group>
                        </Group>

                        <Divider opacity={0.4} />

                        <Stack gap="xs">
                            <Text fw={600} size="sm">
                                Выберите версию для установки:
                            </Text>

                            <Select
                                data={[
                                    {
                                        value: 'stable',
                                        label: `Стабильный релиз (${updateInfo?.latestRelease?.tag_name || 'v3.4.2'})`
                                    },
                                    {
                                        value: 'prerelease',
                                        label: `Pre-release / Beta (${updateInfo?.latestPrerelease?.tag_name || 'v3.5.0-beta.1'})`
                                    },
                                    { value: 'custom', label: 'Выбрать конкретную версию из списка...' },
                                    { value: 'manual', label: 'Ввести версию вручную...' }
                                ]}
                                onChange={(val) => setChannel((val as 'stable' | 'prerelease' | 'custom' | 'manual') || 'stable')}
                                value={channel}
                            />

                            {channel === 'custom' && (
                                <Select
                                    data={releaseOptions}
                                    label="Список релизов (Release & Pre-release)"
                                    onChange={(val) => setSelectedVersion(val || '')}
                                    placeholder="Выберите версию..."
                                    value={selectedVersion}
                                />
                            )}

                            {(channel === 'manual' || selectedVersion === 'manual') && (
                                <TextInput
                                    description="Введите тег релиза (например: v3.5.0-beta.1 или 3.4.16)"
                                    label="Указать версию вручную"
                                    onChange={(e) => setManualVersion(e.currentTarget.value)}
                                    placeholder="v3.5.0-beta.1"
                                    value={manualVersion}
                                />
                            )}

                            <Group gap="xs" mt="xs">
                                <Text c="dimmed" size="xs">
                                    Переход:
                                </Text>
                                <Badge color="gray" size="sm" variant="outline">
                                    v{updateInfo?.currentVersion || 'Текущая'}
                                </Badge>
                                <TbArrowRight size={14} />
                                <Badge
                                    color={effectiveTargetVer.includes('beta') || effectiveTargetVer.includes('rc') || effectiveTargetVer.includes('Pre-release') ? 'orange' : 'blue'}
                                    size="sm"
                                    variant="filled"
                                >
                                    {effectiveTargetVer}
                                </Badge>
                            </Group>
                        </Stack>

                        <Divider opacity={0.4} />

                        <Box>
                            <Group justify="space-between">
                                <Group gap="xs">
                                    <TbCpu size={18} />
                                    <Text fw={500} size="sm">
                                        Кастомное ядро Xray (для энтузиастов)
                                    </Text>
                                </Group>
                                <Switch
                                    checked={isCustomCoreEnabled}
                                    onChange={(e) => setIsCustomCoreEnabled(e.currentTarget.checked)}
                                    size="sm"
                                />
                            </Group>

                            {isCustomCoreEnabled && (
                                <Stack gap="xs" mt="sm">
                                    <Text c="dimmed" size="xs">
                                        Позволяет установить кастомное ядро Xray (форк или тестовый билд) напрямую из GitHub репозитория.
                                    </Text>
                                    <TextInput
                                        description="Репозиторий GitHub для загрузки Xray ядра"
                                        label="GitHub Репозиторий ядра"
                                        onChange={(e) => setCustomCoreRepo(e.currentTarget.value)}
                                        value={customCoreRepo}
                                    />
                                    <TextInput
                                        description="Оставьте пустым для последнего стабильного релиза"
                                        label="Версия ядра (тег)"
                                        onChange={(e) => setCustomCoreVersion(e.currentTarget.value)}
                                        placeholder="v26.3.27"
                                        value={customCoreVersion}
                                    />
                                </Stack>
                            )}
                        </Box>

                        <Divider opacity={0.4} />

                        <Checkbox
                            checked={confirmed}
                            color="orange"
                            label="Я подтверждаю обновление и перезапуск Remnanode"
                            onChange={(e) => setConfirmed(e.currentTarget.checked)}
                            size="sm"
                        />

                        {applyMutation.isSuccess && (
                            <Alert color="teal" icon={<TbCheck size={18} />} title="Успешно">
                                Запрос на обновление отправлен. Служба перезапускается.
                            </Alert>
                        )}

                        <Group justify="flex-end" mt="sm">
                            <Button
                                disabled={applyMutation.isPending}
                                onClick={hide}
                                size="sm"
                                variant="subtle"
                            >
                                Отмена
                            </Button>
                            <Button
                                color="orange"
                                disabled={!confirmed || applyMutation.isPending}
                                leftSection={<TbCloudDownload size={16} />}
                                loading={applyMutation.isPending}
                                onClick={() => applyMutation.mutate()}
                                size="sm"
                            >
                                {applyMutation.isPending ? 'Обновление...' : 'Обновить сейчас'}
                            </Button>
                        </Group>
                    </>
                )}
            </Stack>
        </Modal>
    )
})
