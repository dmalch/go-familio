package familio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
)

// Catalog wire bodies captured from familio.org on 2026-10-08, trimmed (bound
// settlements to the keys the client reads, the catalog to its first fields),
// otherwise as sent — the excerpt endpoint \u-escapes its Cyrillic.
const (
	// a metric-book record (mkkoturkul): Julian birth date, HTML values, two bound settlements.
	wireCatalogPersonMetric = `{"@context":"/contexts/Excerpt","@id":"/catalogs/mkkoturkul/excerpts/f538a502-0cd1-4cf4-bac7-a77f55d90c92","@type":"Excerpt","uuid":"f538a502-0cd1-4cf4-bac7-a77f55d90c92","recordID":"f4111a69-9882-45fd-8d54-5d893abd0ce7","excerptsType":"person","excerptsText":"\u0418\u0432\u0430\u043d\u043e\u0432 \u0424\u043e\u043c\u0430 \u0418\u0432\u0430\u043d\u043e\u0432","attributes":{"birth_date":{"type":"equal","calendar":"julian","first":{"year":1859,"month":7,"day":6,"formatted":"06.07.1859","type":"julian"},"second":null,"formatted":"06.07.1859 \u0441\u0442."},"birth_place":"\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b.","geography":["\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b."]},"record":{"uuid":"f4111a69-9882-45fd-8d54-5d893abd0ce7","position":1483,"record_text":"\u0418\u0432\u0430\u043d\u043e\u0432 \u0424\u043e\u043c\u0430 \u0418\u0432\u0430\u043d\u043e\u0432","sex":"\u041c","type_r":"\u043e \u0440\u043e\u0436\u0434\u0435\u043d\u0438\u0438","record_n":"297-\u0440","role":"\u0423\u0440\u043e\u0436\u0434.","event_date":"06.07.1859","parish":"\u0421\u0432. \u041d\u0438\u043a\u043e\u043b\u0430\u044f \u0427\u0443\u0434\u043e\u0442\u0432\u043e\u0440\u0446\u0430 \u0446.","region":"\u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f","district":"\u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439","location":"\u041a\u043e\u0442\u0443\u0440\u043a\u0443\u043b\u044c","full_location_person":"<a  href=\"https://familio.org/settlements/f4f7d55b-8213-4d52-a15c-a08b4c811052\">\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b.</a>","baptism_date":"07.07.1859","priest":"\u0441\u0432\u044f\u0449\u0435\u043d\u043d\u0438\u043a \u0410\u043b\u0435\u043a\u0441\u0430\u043d\u0434\u0440 \u0410\u043b\u0435\u043a\u0441\u0430\u043d\u0434\u0440\u043e\u0432, \u0434\u044c\u044f\u0447\u0435\u043a \u0418\u0432\u0430\u043d \u0424\u0438\u0440\u0441\u043e\u0432 ","archive_link":"\u0413\u0410\u041e\u041e (\u041e\u043c\u0441\u043a), \u0444.21, \u043e\u043f.2, \u0434.197, \u043b\u0438\u0441\u0442: 725\u043e\u0431","full_record":"\u0421\u0447\u0451\u0442 \u0440\u043e\u0434\u0438\u0432\u0448\u0438\u0445\u0441\u044f: 27 \u043c\u0443\u0436<br>\u0423\u0440\u043e\u0436\u0434. - <b>\u0418\u0432\u0430\u043d\u043e\u0432 \u0424\u043e\u043c\u0430 \u0418\u0432\u0430\u043d\u043e\u0432</b><br>\u041e\u0442\u0435\u0446 - <b>\u0418\u0432\u0430\u043d\u043e\u0432 \u0418\u0432\u0430\u043d \u041b\u0443\u043a\u0438\u0430\u043d\u043e\u0432</b>, \u0420\u0435\u0437\u0435\u0440\u0432\u043d\u044b\u0439 \u041a\u0430\u0437\u0430\u043a  (<a  href=\"https://familio.org/settlements/f4f7d55b-8213-4d52-a15c-a08b4c811052\">\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b.</a>)<br>\u041c\u0430\u0442\u044c - <b>\u0418\u0432\u0430\u043d\u043e\u0432\u0430 \u0414\u0430\u0440\u044c\u044f \u0418\u0432\u0430\u043d\u043e\u0432\u0430</b> (<a  href=\"https://familio.org/settlements/f4f7d55b-8213-4d52-a15c-a08b4c811052\">\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b.</a>)<br>\u0412\u043e\u0441\u043f\u0440. - <b>\u0412\u0430\u0441\u0438\u043b\u0438\u0439 \u041d\u0438\u043a\u043e\u043b\u0430\u0435\u0432 \u0418\u0432\u0430\u043d\u043e\u0432</b>, \u0420\u0435\u0437\u0435\u0440\u0432\u043d\u044b\u0439 \u043a\u0430\u0437\u0430\u043a (\u0441\u0442. \u0429\u0443\u0447\u0438\u043d\u0441\u043a, \u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439 \u0443., \u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f \u043e\u0431\u043b.)<br>\u0412\u043e\u0441\u043f\u0440. - <b>\u041c\u0430\u0442\u0440\u043e\u043d\u0430 \u0425\u0430\u0440\u0438\u0442\u043e\u043d\u043e\u0432\u0430 \u0418\u0432\u0430\u043d\u043e\u0432\u0430</b>, \u041a\u0430\u0437\u0430\u0447\u044c\u044f \u0436\u0435\u043d\u0430 ","list":"<a href=\"/knowledge-base/catalogs/mkkoturkul?page-group=1&page-records=1&type_r=\u043e \u0440\u043e\u0436\u0434\u0435\u043d\u0438\u0438&year=1859&parish=\u0421\u0432. \u041d\u0438\u043a\u043e\u043b\u0430\u044f \u0427\u0443\u0434\u043e\u0442\u0432\u043e\u0440\u0446\u0430 \u0446.&region=\u0410\u043a\u043c\u043e\u043b\u0438\u043d\u0441\u043a\u0430\u044f&distict=\u041a\u043e\u043a\u0447\u0435\u0442\u0430\u0432\u0441\u043a\u0438\u0439&location=\u041a\u043e\u0442\u0443\u0440\u043a\u0443\u043b\u044c&record_n=297-\u0440\">\u041f\u0435\u0440\u0435\u0439\u0442\u0438</a>","source":"\u0418\u043d\u0434\u0435\u043a\u0441\u0430\u0446\u0438\u044f \u0434\u0430\u043d\u043d\u044b\u0445 \u043f\u0440\u043e\u0432\u0435\u0434\u0435\u043d\u0430 <a href=\"/users/950df879-af9a-48de-9fa8-90dd11efa65c\">\u042f\u0440\u043e\u0441\u043b\u0430\u0432\u043e\u043c \u041c\u043e\u0440\u043e\u0437\u043e\u0432\u044b\u043c</a>"},"catalogKey":"mkkoturkul","persons":[],"parishes":[],"settlements":[{"@context":"/contexts/Settlement","@id":"/settlements/8a256826-5cd9-4fb7-be62-66fb1a64e0d8","@type":"Settlement","primaryName":{"settlement":"/settlements/8a256826-5cd9-4fb7-be62-66fb1a64e0d8","language":{"name":"\u0440\u0443\u0441\u0441\u043a\u0438\u0439","code":"ru"},"name":"\u041a\u0430\u0442\u0430\u0440\u043a\u043e\u043b\u044c"},"type":{"name":{"ru":"\u0441\u0435\u043b\u043e"}},"status":{"name":{"ru":"\u0436\u0438\u043b\u043e\u0439"}}},{"@context":"/contexts/Settlement","@id":"/settlements/f4f7d55b-8213-4d52-a15c-a08b4c811052","@type":"Settlement","primaryName":{"settlement":"/settlements/f4f7d55b-8213-4d52-a15c-a08b4c811052","language":{"name":"\u0440\u0443\u0441\u0441\u043a\u0438\u0439","code":"ru"},"name":"\u0429\u0443\u0447\u0438\u043d\u0441\u043a"},"type":{"name":{"ru":"\u0433\u043e\u0440\u043e\u0434"}},"status":{"name":{"ru":"\u0436\u0438\u043b\u043e\u0439"}}}],"birth_date":{"type":"equal","calendar":"julian","first":{"year":1859,"month":7,"day":6,"formatted":"06.07.1859","type":"julian"},"second":null,"formatted":"06.07.1859 \u0441\u0442."},"death_date":null}`

	// a WWI casualty-list record (gwarmil) that two tree persons cite.
	wireCatalogPersonGwarmil = `{"@context":"/contexts/Excerpt","@id":"/catalogs/gwarmil/excerpts/0123e5fb-e298-46e7-8779-a9bfa793ca5a","@type":"Excerpt","uuid":"0123e5fb-e298-46e7-8779-a9bfa793ca5a","recordID":"962d35fa-f533-4291-ba2b-ec55ace55598","excerptsType":"person","excerptsText":"\u0418\u0432\u0430\u043d\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432 ","attributes":{"geography":["\u0422\u043e\u0431\u043e\u043b\u044c\u0441\u043a\u0430\u044f \u0433\u0443\u0431\u0435\u0440\u043d\u0438\u044f, \u041a\u0443\u0440\u0433\u0430\u043d\u0441\u043a\u0438\u0439 \u0443\u0435\u0437\u0434, \u041a\u0440\u0438\u0432\u0438\u043d\u0441\u043a\u0430\u044f \u0432\u043e\u043b\u043e\u0441\u0442\u044c"]},"record":{"uuid":"962d35fa-f533-4291-ba2b-ec55ace55598","position":17290800,"record_text":"\u0418\u0432\u0430\u043d\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432 ","url":"<a href=\"https://gwar.mil.ru/heroes/chelovek_gospital1711350/\" target=\"_blank\" rel=\"nofollow\">https://gwar.mil.ru/heroes/chelovek_gospital1711350/</a>"},"catalogKey":"gwarmil","persons":[{"@context":"/contexts/Person","@id":"/persons/a19694e8-f9a6-47b2-9887-ce26af3ee803","@type":"Person","lastName":"","firstName":"\u041f\u0435\u0440\u0441\u043e\u043d\u0430\u0436\u043a\u0430","middleName":"","sex":"f","biography":null,"birthFirstName":"","birthLastName":"","parents":[],"children":[],"spouse":null,"isVisible":true,"bookmarkedAt":null},{"@context":"/contexts/Person","@id":"/persons/f3e6247a-ab02-4cea-acb4-a52975110241","@type":"Person","lastName":"","firstName":"\u0412\u0430\u043b\u0435\u043d\u0442\u0438\u043d\u0430","middleName":"","sex":"f","biography":"\u0432\u044b\u0432\u044b\u0432\u044b\u0432\u044b\u0432 \u0432\u044b\u0432\u044b\u0432 \u044b\u0432\u044b\u0432\u044b\u0432 \u0432\u044b\u044b\u0432\u044b\u0432","birthFirstName":"","birthLastName":"","parents":[],"children":[],"spouse":null,"isVisible":true,"bookmarkedAt":null}],"parishes":[],"settlements":[],"birth_date":null,"death_date":null}`

	// a record whose attributes are PHP's empty map, [].
	wireCatalogPersonVSS = `{"@context":"/contexts/Excerpt","@id":"/catalogs/vss/excerpts/74f441c3-c698-4cd1-a6f1-a7a9fbc34909","@type":"Excerpt","uuid":"74f441c3-c698-4cd1-a6f1-a7a9fbc34909","recordID":"7532adb6-85ca-4948-aa3f-f3179ecc0188","excerptsType":"person","excerptsText":"\u0418\u0432\u0430\u043d\u043e\u0432 \u042f\u043a\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432","attributes":[],"record":{"uuid":"7532adb6-85ca-4948-aa3f-f3179ecc0188","position":8601110,"record_text":"\u0418\u0432\u0430\u043d\u043e\u0432 \u042f\u043a\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432","surname":"\u0418\u0432\u0430\u043d\u043e\u0432","name":"\u042f\u043a\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432","patronymic":"\u0418\u0432\u0430\u043d\u043e\u0432","source_description_orig":"\u0412\u0435\u0434\u043e\u043c\u043e\u0441\u0442\u044c \u0441\u043f\u0440\u0430\u0432\u043e\u043a \u043e \u0441\u0443\u0434\u0438\u043c\u043e\u0441\u0442\u0438 / \u041c-\u0432\u043e \u044e\u0441\u0442\u0438\u0446\u0438\u0438. - \u0421\u0430\u043d\u043a\u0442-\u041f\u0435\u0442\u0435\u0440\u0431\u0443\u0440\u0433 : \u0422\u0438\u043f. \u041f\u0440\u0430\u0432\u0438\u0442\u0435\u043b\u044c\u0441\u0442\u0432\u0443\u044e\u0449\u0435\u0433\u043e \u0421\u0435\u043d\u0430\u0442\u0430, 1870-1929. - 25-30 \u0441\u043c. 1893, \u043a\u043d. 8, \u0447. 1: \u0421\u043f\u0438\u0441\u043a\u0438 \u043e\u0441\u0443\u0436\u0434\u0435\u043d\u043d\u044b\u0445 \u0432 1893 \u0433\u043e\u0434\u0443. \u0421. 127.","url":"<a href=\"https://viewer.rsl.ru/ru/rsl01005426139?page=64&rotate=0&theme=white\" target=\"_blank\" rel=\"nofollow\">\u0421\u043a\u0430\u043d</a>","original_text":"56743. \u0418\u0412\u0410\u041d\u041e\u0412, \u042f\u043a\u043e\u0432 \u0418\u0432\u0430\u043d\u043e\u0432. \u0421\u0441\u044b\u043b\u044c\u043d\u043e-\u043f\u043e\u0441\u0435\u043b\u0435\u043d. \u0418\u0448\u0438\u043c\u0441\u043a. \u043e\u043a\u0440., \u0427\u0443\u0440\u0442\u0430\u043d\u0441\u043a. \u0432., \u0434. \u0417\u0430\u0431\u043e\u0440\u0441\u043a\u043e\u0439 (\u0447\u0435\u0440\u043d\u043e\u0440\u0430\u0431.), 47 \u043b., \u0440\u043e\u0434. \u0413\u043e\u0440\u043e\u0434\u043e\u043a. \u0443., \u0425\u043e\u043b\u043e\u043c\u0435\u0440\u0441\u043a. \u0432., \u0432 \u0434. \u041c\u0438\u0448\u0443\u0442\u0438\u043d\u043e; \u043f\u0440\u0438\u0433. \u0412\u0438\u0442\u0435\u0431. \u041e\u043a\u0440. \u0421. (\u0432\u043e 2 \u0440., \u043f\u043e \u0440\u0430\u0437\u043b.), \u043f\u043e 2 \u0447. 313 \u0441\u0442. \u0423\u043b\u043e\u0436., \u0432 \u0442\u044e\u0440\u044c\u043c\u0443 \u043d\u0430 2 \u043c. \u0418\u0441\u043f. 29 \u0418\u044e\u043b\u044f 1893 \u0433."},"catalogKey":"vss","persons":[],"parishes":[],"settlements":[{"@context":"/contexts/Settlement","@id":"/settlements/4b96860e-5d93-43e0-9c32-87a99a03f328","@type":"Settlement","primaryName":{"settlement":"/settlements/4b96860e-5d93-43e0-9c32-87a99a03f328","language":{"name":"\u0440\u0443\u0441\u0441\u043a\u0438\u0439","code":"ru"},"name":"\u041c\u0438\u0448\u0443\u0442\u0438\u043d\u043e"},"type":{"name":{"ru":"\u0434\u0435\u0440\u0435\u0432\u043d\u044f"}},"status":{"name":{"ru":"\u0436\u0438\u043b\u043e\u0439"}}},{"@context":"/contexts/Settlement","@id":"/settlements/6adfd6cb-6e0b-4d91-81f1-99b7e21e4925","@type":"Settlement","primaryName":{"settlement":"/settlements/6adfd6cb-6e0b-4d91-81f1-99b7e21e4925","language":{"name":"\u0440\u0443\u0441\u0441\u043a\u0438\u0439","code":"ru"},"name":"\u0412\u0438\u0442\u0435\u0431\u0441\u043a"},"type":{"name":{"ru":"\u0433\u043e\u0440\u043e\u0434"}},"status":{"name":{"ru":"\u0436\u0438\u043b\u043e\u0439"}}}],"birth_date":null,"death_date":null}`

	// the metric-book catalog, trimmed to its first 8 fields; this endpoint sends raw UTF-8.
	wireCatalogMkkoturkul = `{"@context":"/contexts/Catalog","@id":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","@type":"Catalog","name":"Метрические книги станицы Котуркульской за 1854-1876 годы","hidden":false,"key":"mkkoturkul","description":"Метрические книги станицы Котуркуль, Кокчетавский уезд, Акмолинская область за 1854,1855, 1858, 1859, 1860, 1876 годы. В приход входили станицы Котуркульская и Щучинская. В ряде случаев вместо конкретного НП указана лишь волость - тогда человек привязывался к волостному центру. ","issueYearDescription":"1854-1876","catalogType":"fundSettlements","catalogsFields":[{"key":"record_text","title":"ФИО","type":"string","nullable":true,"showInTable":true,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":100,"system":true,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"466c2cdf-f901-408f-ae43-1e796ffd60e0"},{"key":"sex","title":"Пол","type":"string","nullable":true,"showInTable":false,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":200,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"784cffb8-9e1e-4d01-bdf9-313e6c29faf2"},{"key":"type_r","title":"Тип записи","type":"string","nullable":true,"showInTable":true,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":300,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"970838a7-9ecc-4df4-ba1b-50dea5ec5e4c"},{"key":"record_n","title":"Номер в справочнике","type":"string","nullable":true,"showInTable":true,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":400,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2024-12-07T17:57:43+00:00","uuid":"cd8c8dfb-0744-43ba-af25-a106187ce815"},{"key":"role","title":"Роль персоны","type":"string","nullable":true,"showInTable":true,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":500,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"32fef8d1-7c5d-46dc-b9a4-df910a5d7822"},{"key":"event_date","title":"Дата события","type":"string","nullable":true,"showInTable":false,"showInCard":true,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":600,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"a17fa187-2457-409e-968b-d5b45fdd4ba9"},{"key":"year","title":"Год","type":"string","nullable":true,"showInTable":true,"showInCard":false,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":700,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"26c573ae-92fe-499b-99d8-dc14d500ec13"},{"key":"month","title":"Месяц","type":"string","nullable":true,"showInTable":true,"showInCard":false,"catalog":"/catalogs/87d0a957-7ccf-431e-b092-7abd673fff3d","sortable":true,"userOrder":800,"system":false,"hiddenFromUnauthorized":false,"createdAt":"2023-09-11T07:53:49+00:00","updatedAt":"2023-09-11T07:53:49+00:00","uuid":"b65b4109-d353-4a54-878f-af492f479986"}],"recordsCount":4414,"uuid":"87d0a957-7ccf-431e-b092-7abd673fff3d"}`

	// an unknown record: a 404 in v1's error shape.
	wireCatalogPersonMissing = `{"errors":["\u0412\u044b\u043f\u0438\u0441\u043a\u0430 \u043d\u0435 \u043d\u0430\u0439\u0434\u0435\u043d\u0430"]}`
)

// TestGetCatalogPersonDecodesAMetricBookRecord covers the richest record kind:
// structured Julian dates, the catalog-specific record fields with their HTML,
// the attributes' geography and birth place, and the bound settlements.
func TestGetCatalogPersonDecodesAMetricBookRecord(t *testing.T) {
	RegisterTestingT(t)
	var got *http.Request
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = io.WriteString(w, wireCatalogPersonMetric)
	})
	defer srv.Close()

	p, err := newTestClient(srv).GetCatalogPerson(context.Background(), "mkkoturkul", "f538a502-0cd1-4cf4-bac7-a77f55d90c92")

	Expect(err).ToNot(HaveOccurred())
	Expect(got.URL.Path).To(Equal("/api/v1/catalogs/mkkoturkul/excerpts/f538a502-0cd1-4cf4-bac7-a77f55d90c92"))
	Expect(got.Header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
	Expect(p.UUID).To(Equal("f538a502-0cd1-4cf4-bac7-a77f55d90c92"))
	Expect(p.CatalogKey).To(Equal("mkkoturkul"))
	Expect(p.RecordID).ToNot(BeEmpty())
	Expect(p.Text).To(ContainSubstring("Иванов"))
	Expect(p.BirthDate).ToNot(BeNil())
	Expect(p.BirthDate.Calendar).To(Equal("julian"))
	Expect(p.BirthDate.First.Year).To(Equal(1859))
	Expect(p.DeathDate).To(BeNil())
	Expect(p.Geography).To(Equal([]string{"ст. Щучинск, Кокчетавский у., Акмолинская обл."}))
	Expect(p.BirthPlace).To(Equal("ст. Щучинск, Кокчетавский у., Акмолинская обл."))
	Expect(p.Attributes).To(HaveKey("birth_date"))
	Expect(p.Record).To(HaveKeyWithValue("baptism_date", "07.07.1859"))
	Expect(p.Record["full_record"]).To(ContainSubstring("<br>"), "record values are kept as sent, HTML included")
	Expect(p.Settlements).To(HaveLen(2))
	Expect(p.Settlements[0].UUID).ToNot(BeEmpty())
	Expect(p.Settlements[0].Name).To(Equal("Катарколь"))
	Expect(p.Settlements[0].Type).To(Equal("село"))
	Expect(p.LinkedPersons).To(BeEmpty())
}

// TestGetCatalogPersonDecodesLinkedPersons covers the tree persons that cite a
// record, and familio's one-letter sex code.
func TestGetCatalogPersonDecodesLinkedPersons(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, wireCatalogPersonGwarmil)
	})
	defer srv.Close()

	p, err := newTestClient(srv).GetCatalogPerson(context.Background(), "gwarmil", "0123e5fb-e298-46e7-8779-a9bfa793ca5a")

	Expect(err).ToNot(HaveOccurred())
	Expect(p.Geography).To(Equal([]string{"Тобольская губерния, Курганский уезд, Кривинская волость"}))
	Expect(p.Record).To(HaveKey("url"))
	Expect(p.BirthDate).To(BeNil())
	Expect(p.LinkedPersons).To(HaveLen(2))
	Expect(p.LinkedPersons[0]).To(Equal(CatalogLinkedPerson{
		UUID: "a19694e8-f9a6-47b2-9887-ce26af3ee803", FirstName: "Персонажка", Gender: GenderFemale,
	}))
}

// TestGetCatalogPersonEmptyAttributes covers PHP's empty map: attributes come
// as [] when a record has none, and must still decode.
func TestGetCatalogPersonEmptyAttributes(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, wireCatalogPersonVSS)
	})
	defer srv.Close()

	p, err := newTestClient(srv).GetCatalogPerson(context.Background(), "vss", "74f441c3-c698-4cd1-a6f1-a7a9fbc34909")

	Expect(err).ToNot(HaveOccurred())
	Expect(p.Attributes).To(BeEmpty())
	Expect(p.Geography).To(BeEmpty())
	Expect(p.Record).To(HaveKey("original_text"))
	Expect(p.Settlements).ToNot(BeEmpty())
}

// TestGetCatalogDecodesFields covers the catalog and its field metadata, whose
// titles label a record's catalog-specific keys, in display order.
func TestGetCatalogDecodesFields(t *testing.T) {
	RegisterTestingT(t)
	var got *http.Request
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = io.WriteString(w, wireCatalogMkkoturkul)
	})
	defer srv.Close()

	c, err := newTestClient(srv).GetCatalog(context.Background(), "mkkoturkul")

	Expect(err).ToNot(HaveOccurred())
	Expect(got.URL.Path).To(Equal("/api/v1/catalogs/mkkoturkul"))
	Expect(c.Key).To(Equal("mkkoturkul"))
	Expect(c.UUID).To(Equal("87d0a957-7ccf-431e-b092-7abd673fff3d"))
	Expect(c.Name).To(HavePrefix("Метрические книги станицы Котуркульской"))
	Expect(c.Years).ToNot(BeEmpty())
	Expect(c.Type).To(Equal("fundSettlements"))
	Expect(c.RecordsCount).To(BeNumerically(">", 0))
	Expect(c.Fields).To(HaveLen(8))
	Expect(c.Fields[0]).To(Equal(CatalogField{
		Key: "record_text", Title: "ФИО", Type: "string", ShowInCard: true, ShowInTable: true,
	}))
}

// TestGetCatalogPersonMissingIsNotFound covers an unknown record: a 404 in v1's
// {"errors": […]} shape.
func TestGetCatalogPersonMissingIsNotFound(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, wireCatalogPersonMissing)
	})
	defer srv.Close()

	_, err := newTestClient(srv).GetCatalogPerson(context.Background(), "gwarmil", "00000000-0000-0000-0000-000000000000")

	Expect(err).To(MatchError(ErrNotFound))
	Expect(err.Error()).To(ContainSubstring("Выписка не найдена"))
}

// TestCatalogReadsRejectBadIDsUpFront covers the input familio would answer
// with a 500 — which the client retries — or a confusing route: it is
// ErrInvalidRequest before any request.
func TestCatalogReadsRejectBadIDsUpFront(t *testing.T) {
	RegisterTestingT(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()
	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	ctx := context.Background()

	_, err := client.GetCatalogPerson(ctx, "gwarmil", "not-a-uuid")
	Expect(err).To(MatchError(ErrInvalidRequest))
	_, err = client.GetCatalogPerson(ctx, "", "774b6dcb-b44a-4304-944b-d7a5d4513c61")
	Expect(err).To(MatchError(ErrInvalidRequest))
	_, err = client.GetCatalogPerson(ctx, "gw/armil", "774b6dcb-b44a-4304-944b-d7a5d4513c61")
	Expect(err).To(MatchError(ErrInvalidRequest))
	_, err = client.GetCatalog(ctx, "")
	Expect(err).To(MatchError(ErrInvalidRequest))
	Expect(hits.Load()).To(BeZero())
}

// TestCatalogReadsAreAnonymousWithoutSession checks the catalog reads are
// public: no `t` cookie, no bearer and no token scrape.
func TestCatalogReadsAreAnonymousWithoutSession(t *testing.T) {
	RegisterTestingT(t)
	var scraped atomic.Bool
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			scraped.Store(true)
			return
		}
		auth.Store(r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, wireCatalogPersonGwarmil)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	_, err := client.GetCatalogPerson(context.Background(), "gwarmil", "0123e5fb-e298-46e7-8779-a9bfa793ca5a")

	Expect(err).ToNot(HaveOccurred())
	Expect(auth.Load()).To(Equal(""))
	Expect(scraped.Load()).To(BeFalse())
}
