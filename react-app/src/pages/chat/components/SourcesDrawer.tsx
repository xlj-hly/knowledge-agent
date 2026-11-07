import { Drawer } from 'antd'
import { Link } from 'react-router'
import type { Message, Source } from '../../../api/chat/types'
import { Icon } from '../../../components/ui'

export function SourcesDrawer({ message, onClose }: { message: Message | undefined; onClose: () => void }) {
  const groups = new Map<string, { title: string; base: string; items: { source: Source; index: number }[] }>()
  message?.sources.forEach((source, index) => {
    const key = `${source.base_id}:${source.document_id}`
    const group = groups.get(key) ?? { title: source.document_title, base: source.base_id, items: [] }
    group.items.push({ source, index: index + 1 }); groups.set(key, group)
  })
  return <Drawer title="参考资料" open={Boolean(message)} onClose={onClose} size={420} destroyOnHidden>
    <p className="chat-source-intro">本次回答引用的原文片段</p>
    <div className="chat-source-list">{[...groups.entries()].map(([key, group]) => <section key={key} className="chat-source-document"><h3><Icon name="file" size={17} />{group.title}</h3><Link to={`/knowledge/${group.base}`} onClick={onClose}>查看知识库 <Icon name="arrow" size={13} /></Link>{group.items.map(({ source, index }) => <div className="chat-source-excerpt" key={`${source.chunk_id}:${index}`}><span>{index}. {source.document_title}</span><p>{source.content}</p></div>)}</section>)}</div>
  </Drawer>
}
