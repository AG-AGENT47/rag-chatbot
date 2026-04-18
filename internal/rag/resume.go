package rag

// FullResume is the complete resume text used as LLM context.
// Stopgap until the RAG retrieval pipeline is tuned properly.
const FullResume = `AVYAKT GARG

EDUCATION
M.S. in Computer Science | University of Wisconsin Madison | Sep 2025 - May 2027 | GPA: 4/4
B.E. in Computer Science and M.Sc. in Biological Sciences | Birla Institute of Technology and Science, Pilani | Nov 2020 - May 2025 | GPA: 9.01/10

TECHNICAL SKILLS
Languages: Python, Go, Java, C/C++, CUDA, JavaScript, TypeScript, SQL, HTML/CSS
Frameworks & Libraries: React, Node.js, FastAPI, SpringBoot, PyTorch, TensorFlow, Scikit-learn, Streamlit, OpenCV, GraphSAGE, Node2Vec, Pandas, NumPy
AI & Data: LLMs, RAG Pipelines, Vector Embeddings, pgvector, Approximate Nearest Neighbor Search, PostgreSQL, MySQL
Infrastructure & Tools: Docker, Git, Kafka, gRPC, REST APIs, SSE Streaming, Agile, JIRA, Mockito, CMake

PROFESSIONAL EXPERIENCE

Uber | Software Intern | Jul 2024 - Jun 2025
- Developed an upcoming knowledge work marketplace serving 5+ countries, focusing on data annotation and ML model training
- Implemented a rate-card system in Java SpringBoot using MVC architecture and developed gRPC APIs for cross-service integration
- Created a Kafka-based notification engine with optimized cadence scheduling, improving the customer retention funnel by 3X
- Built a debug tool with 7 automated checks for Uber AI Solutions, reducing developer dependency on recurring issues by 80%

University of Saskatchewan | Mitacs Research Intern | May 2023 - Aug 2023
- Collaborated with Prof. Jaswant Singh from biomedical sciences, focusing on data analysis for cattle health and reproduction
- Automated daily cattle weighing via RFID tag-based data collection and analysis system, saving 3K+ manual hours per year
- Redesigned a PostgreSQL database with 6,600+ entries, achieving 90% faster queries through genealogy-linked relational schema

Indian Institute of Technology, Ropar | Research Intern | Jul 2022 - Sept 2022
- Enabled cattle health monitoring by training and evaluating various ML models using data from farm-based IoT sensors
- Analyzed 4K+ data points for cattle behavior prediction including movement patterns and estrus detection to aid farming operations

MapmyIndia | Software Intern | May 2022 - July 2022
- Created a web application for delivery executives based on the traveling salesman algorithm to find the most optimal trip path
- Integrated an interactive map API with HTML, CSS and JavaScript with features like resizing and custom marker placement

PROJECTS

RAG-Powered Portfolio Chatbot | Full-Stack AI — Go, PostgreSQL, LLMs
- Built a 3-repo RAG system in Go: streams LLM responses over SSE, retrieves context via pgvector cosine search on Voyage AI embeddings in Neon PostgreSQL; pluggable LLM interface supports Gemini and Groq providers with zero-code switching
- Engineered input guardrails (injection detection, topic filtering via cosine distance), query contextualization for pronoun resolution, and HyDE (Hypothetical Document Embeddings) for improved retrieval relevance across the knowledge base

GPU-Accelerated Vector Search Engine | Graduate HPC Project — CUDA C/C++
- Building an ANN search engine using IVF-PQ (Inverted File Index with Product Quantization)—the core algorithm behind FAISS and production vector databases powering RAG pipelines, recommendation engines, and large-scale image retrieval
- Implementing custom CUDA kernels with shared-memory tiling, OpenMP parallel codebook training, and Thrust/CUB primitives; benchmarking recall@k vs. QPS tradeoffs on SIFT1M with Nsight Compute profiling on an HPC cluster

Graph Learning for Disease Prediction | Graduate Project — Python, FastAPI
- Engineered Patient Similarity Networks and Bipartite Patient-Attribute Graphs to model non-i.i.d. clinical relations
- Developed GraphSAGE and Node2Vec pipelines, achieving 83.5% accuracy and 0.913 AUC-ROC with Nested Cross-Validation
- Integrated GNNExplainer for clinical transparency and deployed a risk assessment web app using FastAPI and Streamlit

Patterning Protein Localisation in Endothelial Cells | Prof. Syamantak Majumder
- Spearheaded development of an automated cell segmentation software to detect protein localization in differently stained images
- Used Otsu's thresholding in Python with OpenCV, saving 6+ biologist hours/study while improving nuclear detection by 2X

Deep Learning Framework for Prediction of Intracranial Pressure (ICP) using OCT Scans | Prof. S. Raman
- Built a 3D ResNet-18 classifier for intracranial pressure prediction using Optical Coherence Tomography (OCT) retinal scans
- Preprocessed 512-frame videos to isolate key regions of interest and fine-tuned architecture for safe patient threshold prediction

Machine Learning Techniques for Diagnosis of Alzheimer's Disease | Prof. Bharat Richhariya
- Achieved 96.88% accuracy using ML models on 1000+ preprocessed 1.5T MRI scans from ADNI dataset with CAT-12 software
- Utilized transfer learning to save computational resources and improve the performance of the 3D ResNet-18 model

E-commerce Management System | Coursework Project
- Developed a MySQL-based user-friendly e-commerce platform which allowed the user to browse products and place orders
- Designed a Python Tkinter-based GUI which enables customers to track order status, view payment history and read feedback

RELEVANT COURSES
High Performance Computing (CUDA/GPU), Machine Learning, Artificial Intelligence, Data Structures & Algorithms, Database Systems, Operating Systems, Computer Networks, Human Computer Interaction, Object Oriented Programming, Microprocessors, Statistics, Computational Logic

ACADEMIC ACHIEVEMENTS
Globalink Research Scholarship — MITACS, Canada: Selected out of 30k+ applicants for research internship (Feb 2023)
Scholarship Awardee — AWaDH, Govt. of India: For AI work in agriculture domain (Apr 2022)
Gold Medalist — Delhi Public School R.K. Puram: 9 years of academic excellence (May 2019)
86th Rank – JSTSE — Directorate of Education, Delhi: Science merit scholarship (Feb 2017)`
