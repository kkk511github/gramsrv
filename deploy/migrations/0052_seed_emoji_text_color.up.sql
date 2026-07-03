WITH seed_emoji_documents AS (
    SELECT DISTINCT jsonb_array_elements_text(document_ids)::bigint AS document_id
    FROM public.sticker_sets
    WHERE set_kind = 'emoji'
      AND COALESCE(creator_user_id, 0) = 0
),
rewritten_documents AS (
    SELECT d.id,
           jsonb_agg(
               CASE
                   WHEN attr.value ->> 'kind' = 'custom_emoji'
                    AND attr.value ->> 'text_color' = 'true'
                   THEN attr.value - 'text_color'
                   ELSE attr.value
               END
               ORDER BY attr.ordinality
           ) AS attributes
    FROM public.documents d
    JOIN seed_emoji_documents seed_docs ON seed_docs.document_id = d.id
    CROSS JOIN LATERAL jsonb_array_elements(d.attributes) WITH ORDINALITY AS attr(value, ordinality)
    WHERE d.attributes @> '[{"kind":"custom_emoji","text_color":true}]'::jsonb
    GROUP BY d.id
)
UPDATE public.documents d
SET attributes = rewritten_documents.attributes
FROM rewritten_documents
WHERE d.id = rewritten_documents.id;

UPDATE public.sticker_sets
SET text_color = false
WHERE set_kind = 'emoji'
  AND COALESCE(creator_user_id, 0) = 0
  AND text_color = true;
