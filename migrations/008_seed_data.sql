-- Seed default knowledge categories
INSERT INTO knowledge_categories (name, is_system, created_at) VALUES
    ('学习资料', TRUE, CURRENT_TIMESTAMP),
    ('项目文档', TRUE, CURRENT_TIMESTAMP),
    ('工具软件', TRUE, CURRENT_TIMESTAMP),
    ('其他资源', TRUE, CURRENT_TIMESTAMP)
ON CONFLICT DO NOTHING;

-- Seed default settings
INSERT INTO settings (key, value) VALUES
    ('site_name', 'SWPUCAT'),
    ('site_description', '软件工作室成员管理平台')
ON CONFLICT (key) DO NOTHING;
